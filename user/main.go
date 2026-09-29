// 用户服务进程入口。
package main

import (
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/user/userservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
)

func main() {
	app := bootstrap.Init(consts.DBUser)
	defer app.Close()

	opts, addr, err := rpcx.ServerOptions(app.Cfg, consts.ServiceUser)
	if err != nil {
		klog.Fatalf("build server options failed: %v", err)
	}

	srv := userservice.NewServer(NewHandler(app.DB, app.Redis), opts...)
	klog.Infof("user service listening on %s", addr)
	if err = srv.Run(); err != nil {
		klog.Fatalf("user service exit: %v", err)
	}
}
