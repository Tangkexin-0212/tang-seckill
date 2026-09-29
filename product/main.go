// 商品服务进程入口。
package main

import (
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/product/productservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
)

func main() {
	app := bootstrap.Init(consts.DBProduct)
	defer app.Close()

	opts, addr, err := rpcx.ServerOptions(app.Cfg, consts.ServiceProduct)
	if err != nil {
		klog.Fatalf("build server options failed: %v", err)
	}

	srv := productservice.NewServer(NewHandler(app.DB), opts...)
	klog.Infof("product service listening on %s", addr)
	if err = srv.Run(); err != nil {
		klog.Fatalf("product service exit: %v", err)
	}
}
