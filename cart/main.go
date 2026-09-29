// 购物车服务进程入口。
package main

import (
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/cart/cartservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
)

func main() {
	// 购物车不使用 MySQL，数据全在 Redis Hash 中
	app := bootstrap.Init("")
	defer app.Close()

	clients, err := rpcx.NewClients(app.Cfg)
	if err != nil {
		klog.Fatalf("init clients failed: %v", err)
	}

	opts, addr, err := rpcx.ServerOptions(app.Cfg, consts.ServiceCart)
	if err != nil {
		klog.Fatalf("build server options failed: %v", err)
	}

	srv := cartservice.NewServer(NewHandler(app.Redis, clients.Product, app.Cfg), opts...)
	klog.Infof("cart service listening on %s", addr)
	if err = srv.Run(); err != nil {
		klog.Fatalf("cart service exit: %v", err)
	}
}
