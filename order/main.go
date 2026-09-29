// 订单服务进程入口。
package main

import (
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/order/orderservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
)

func main() {
	app := bootstrap.Init(consts.DBOrder)
	defer app.Close()

	opts, addr, err := rpcx.ServerOptions(app.Cfg, consts.ServiceOrder)
	if err != nil {
		klog.Fatalf("build server options failed: %v", err)
	}

	srv := orderservice.NewServer(NewHandler(app.DB, app.Redis, app.Cfg), opts...)
	klog.Infof("order service listening on %s", addr)
	if err = srv.Run(); err != nil {
		klog.Fatalf("order service exit: %v", err)
	}
}
