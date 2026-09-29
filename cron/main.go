// 定时任务进程入口（cobra + robfig/cron）。
//
// 两条子命令之外，默认 `run` 会常驻调度：
//
//	sync      每 30s 把 MySQL 中的活动同步进 Redis（预热）
//	reconcile 每 1min 核对 Redis 与 MySQL 的库存是否最终一致
//	cleanup   跟随 reconcile 清理已结束活动的 Redis 残留
//
// 手动执行单次任务：seckill-cron sync / seckill-cron reconcile
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"seckill/kitex_gen/product/productservice"
	"seckill/kitex_gen/stock/stockservice"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/logger"
	"seckill/pkg/redisx"
	"seckill/pkg/rpcx"
	"seckill/pkg/seckillkit"
	"seckill/pkg/utils"
)

// jobTimeout 单个任务的超时时间，避免任务卡死拖垮调度器。
const jobTimeout = 30 * time.Second

var cfgPath string

func main() {
	root := &cobra.Command{
		Use:   "seckill-cron",
		Short: "秒杀活动定时同步与对账服务",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScheduler(cmd)
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath, "配置文件路径")
	root.AddCommand(newSyncCmd(), newReconcileCmd(), newCleanupCmd())

	if err := root.Execute(); err != nil {
		klog.Errorf("cron exit with error: %v", err)
		os.Exit(1)
	}
}

// env 任务运行环境。
type env struct {
	cfg  *config.Config
	rdb  *redisx.Client
	jobs *Jobs
}

// build 加载配置并构造 Redis / RPC 依赖。
// 之所以不复用 bootstrap.Init：cobra 自己做命令行解析，与标准库 flag.Parse 会冲突。
func build() (*env, func(), error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, err
	}
	logger.Init("info")

	rdb, err := redisx.New(cfg.Redis)
	if err != nil {
		return nil, nil, err
	}

	opts, err := rpcx.ClientOptions(cfg)
	if err != nil {
		_ = rdb.Close()
		return nil, nil, err
	}
	product, err := productservice.NewClient(consts.ServiceProduct, opts...)
	if err != nil {
		_ = rdb.Close()
		return nil, nil, err
	}
	stock, err := stockservice.NewClient(consts.ServiceStock, opts...)
	if err != nil {
		_ = rdb.Close()
		return nil, nil, err
	}

	e := &env{
		cfg:  cfg,
		rdb:  rdb,
		jobs: NewJobs(cfg, rdb, seckillkit.NewSyncer(rdb, cfg), product, stock),
	}
	return e, func() { _ = rdb.Close() }, nil
}

// runScheduler 常驻调度。
func runScheduler(cmd *cobra.Command) error {
	e, closeFn, err := build()
	if err != nil {
		return err
	}
	defer closeFn()

	c := cron.New(cron.WithSeconds())
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1) 活动同步：预热 + 状态刷新
	if _, err = c.AddFunc(e.cfg.Cron.SyncSpec, func() {
		jctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		if _, serr := e.jobs.SyncOnce(jctx); serr != nil {
			klog.Errorf("scheduled sync failed: %v", serr)
		}
	}); err != nil {
		return err
	}

	// 2) 对账 + 清理
	if _, err = c.AddFunc(e.cfg.Cron.ReconcileSpec, func() {
		jctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		if _, rerr := e.jobs.ReconcileOnce(jctx, false); rerr != nil {
			klog.Errorf("scheduled reconcile failed: %v", rerr)
		}
		if _, cerr := e.jobs.CleanupExpired(jctx, utils.NowMs()); cerr != nil {
			klog.Errorf("scheduled cleanup failed: %v", cerr)
		}
	}); err != nil {
		return err
	}

	// 3) 启动时立即预热一次，避免等服务启动后要空等一个周期
	if e.cfg.Cron.WarmUpOnStart {
		wctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		if _, serr := e.jobs.SyncOnce(wctx); serr != nil {
			klog.Errorf("warm up on start failed: %v", serr)
		}
		cancel()
	}

	c.Start()
	klog.Infof("cron scheduler started: sync=%s reconcile=%s", e.cfg.Cron.SyncSpec, e.cfg.Cron.ReconcileSpec)

	<-ctx.Done()
	klog.Info("cron scheduler stopping...")
	cStopCtx := c.Stop()
	select {
	case <-cStopCtx.Done():
	case <-time.After(3 * time.Second):
	}
	return nil
}

// newSyncCmd 手动执行一次同步。
func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "执行一次 MySQL → Redis 活动同步",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, closeFn, err := build()
			if err != nil {
				return err
			}
			defer closeFn()

			ctx, cancel := context.WithTimeout(cmd.Context(), jobTimeout)
			defer cancel()

			n, err := e.jobs.SyncOnce(ctx)
			if err != nil {
				return err
			}
			klog.Infof("manual sync finished: synced=%d", n)
			return nil
		},
	}
}

// newReconcileCmd 手动执行一次对账，--fix 时修复异常活动。
func newReconcileCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "执行一次 Redis 与 MySQL 的库存对账",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, closeFn, err := build()
			if err != nil {
				return err
			}
			defer closeFn()

			ctx, cancel := context.WithTimeout(cmd.Context(), jobTimeout)
			defer cancel()

			results, err := e.jobs.ReconcileOnce(ctx, fix)
			if err != nil {
				return err
			}
			for _, r := range results {
				klog.Infof("activity=%d total=%d redisRemain=%d dbDeducted=%d expected=%d consistent=%v",
					r.ActivityID, r.TotalStock, r.RedisRemain, r.DBDeducted, r.ExpectedRemain, r.Consistent)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "对异常活动（DB 扣减多于 Redis）强制以 DB 为准回写库存")
	return cmd
}

// newCleanupCmd 手动清理过期活动。
func newCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "清理已结束活动的 Redis 残留",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, closeFn, err := build()
			if err != nil {
				return err
			}
			defer closeFn()

			ctx, cancel := context.WithTimeout(cmd.Context(), jobTimeout)
			defer cancel()

			n, err := e.jobs.CleanupExpired(ctx, utils.NowMs())
			if err != nil {
				return err
			}
			klog.Infof("manual cleanup finished: removed=%d", n)
			return nil
		},
	}
}
