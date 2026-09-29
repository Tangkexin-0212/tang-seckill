// Package bootstrap 统一各服务的启动流程（配置 / 日志 / MySQL / Redis）。
package bootstrap

import (
	"flag"

	"github.com/cloudwego/kitex/pkg/klog"
	"gorm.io/gorm"

	"seckill/pkg/config"
	"seckill/pkg/logger"
	"seckill/pkg/mysqlx"
	"seckill/pkg/redisx"
)

// App 服务运行时依赖集合。
type App struct {
	Cfg   *config.Config
	DB    *gorm.DB
	Redis *redisx.Client
}

// InitFlags 解析公共命令行参数并返回配置文件路径。
func InitFlags() string {
	cfgPath := flag.String("config", config.DefaultPath, "配置文件路径")
	logLevel := flag.String("log", "info", "日志级别 debug/info/warn/error")
	flag.Parse()
	logger.Init(*logLevel)
	return *cfgPath
}

// Init 加载配置并建立 MySQL / Redis 连接。
// dbName 为空时跳过 MySQL（网关、购物车、定时任务同步等场景不需要 DB 时可传空）。
func Init(dbName string) *App {
	cfg := config.MustLoad(InitFlags())

	app := &App{Cfg: cfg}

	if dbName != "" {
		db, err := mysqlx.Open(cfg.MySQL, dbName)
		if err != nil {
			klog.Fatalf("init mysql(%s) failed: %v", dbName, err)
		}
		app.DB = db
		klog.Infof("mysql(%s) connected", dbName)
	}

	rdb, err := redisx.New(cfg.Redis)
	if err != nil {
		klog.Fatalf("init redis failed: %v", err)
	}
	app.Redis = rdb
	klog.Infof("redis(%s) connected", cfg.Redis.Addr)

	return app
}

// Close 释放资源。
func (a *App) Close() {
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	if a.DB != nil {
		if sqlDB, err := a.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}
