// 异步补偿进程入口，承担两件事：
//
//  1. Stream 消费：把 Redis 秒杀事件落进 MySQL（stock_flow 流水 + 库存扣减）
//  2. 延时扫描：把超时未支付的订单释放掉，库存与购买资格一并归还
//
// 这两件事都不在用户的同步请求链路上，因此独立成进程，
// 挂了也只影响"最终一致"的收敛速度，不会让抢购接口报错。
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/cart/cartservice"
	"seckill/kitex_gen/order/orderservice"
	"seckill/kitex_gen/stock/stockservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
	"seckill/pkg/seckillkit"
)

func main() {
	// worker 需要写 mq_consume_log，因此连接 order_db
	app := bootstrap.Init(consts.DBOrder)
	defer app.Close()

	stock, cart, order, err := newClients(app)
	if err != nil {
		klog.Fatalf("build rpc clients failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	consumer := NewStreamConsumer(app.Cfg, app.Redis, app.DB, stock)
	scanner := NewDelayScanner(app.Redis, seckillkit.NewReleaser(app.Redis, app.Cfg, order, cart))

	go consumer.Run(ctx)
	// 兜底：接管崩溃消费者遗留的 pending 消息
	go consumer.RunClaimLoop(ctx)
	go scanner.Run(ctx, time.Duration(app.Cfg.Order.DelayScanIntervalMs)*time.Millisecond)

	klog.Info("worker started: stream consumer + delay scanner")
	<-ctx.Done()
	klog.Info("worker stopping...")

	// 给在途的落库请求一点收尾时间
	time.Sleep(300 * time.Millisecond)
}

func newClients(app *bootstrap.App) (stockservice.Client, cartservice.Client, orderservice.Client, error) {
	opts, err := rpcx.ClientOptions(app.Cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	var (
		stock stockservice.Client
		cart  cartservice.Client
		order orderservice.Client
	)
	if stock, err = stockservice.NewClient(consts.ServiceStock, opts...); err != nil {
		return nil, nil, nil, err
	}
	if cart, err = cartservice.NewClient(consts.ServiceCart, opts...); err != nil {
		return nil, nil, nil, err
	}
	if order, err = orderservice.NewClient(consts.ServiceOrder, opts...); err != nil {
		return nil, nil, nil, err
	}
	return stock, cart, order, nil
}
