// 库存服务进程入口。
package main

import (
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/stock/stockservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
)

func main() {
	app := bootstrap.Init(consts.DBStock)
	defer app.Close()

	opts, addr, err := rpcx.ServerOptions(app.Cfg, consts.ServiceStock)
	if err != nil {
		klog.Fatalf("build server options failed: %v", err)
	}

	srv := stockservice.NewServer(NewHandler(app.DB), opts...)
	klog.Infof("stock service listening on %s", addr)
	if err = srv.Run(); err != nil {
		klog.Fatalf("stock service exit: %v", err)
	}
}
