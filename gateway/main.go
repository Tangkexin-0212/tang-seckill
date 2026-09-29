// 网关进程入口：Hertz HTTP 服务，统一鉴权 + 参数校验 + RPC 转发。
//
// 客户端只和网关打交道，内部服务全部走 Kitex RPC + etcd 服务发现，
// 网关不直接访问 MySQL/Redis，也不做任何业务状态存储。
package main

import (
	"context"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/pkg/bootstrap"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
)

func main() {
	app := bootstrap.Init("")
	defer app.Close()

	clients, err := rpcx.NewClients(app.Cfg)
	if err != nil {
		klog.Fatalf("build rpc clients failed: %v", err)
	}

	gw := NewGateway(app.Cfg, clients)

	node := app.Cfg.ServerOf(consts.ServiceGateway)
	h := server.Default(server.WithHostPorts(node.Addr()))
	registerRoutes(h, gw, clients)

	klog.Infof("gateway listening on %s", node.Addr())
	h.Spin()
}

// registerRoutes 注册全部 HTTP 路由。
func registerRoutes(h *server.Hertz, gw *Gateway, cli *rpcx.Clients) {
	h.GET("/health", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(http.StatusOK, Resp{Code: 0, Msg: "ok"})
	})

	api := h.Group("/api")

	// 免登录
	api.POST("/user/register", gw.Register)
	api.POST("/user/login", gw.Login)
	api.GET("/product/list", gw.ListProducts)
	api.GET("/activity/list", gw.ListActivities)
	api.GET("/seckill/info", gw.GetSeckillInfo)

	// 需要登录：鉴权中间件把 uid 注入上下文
	auth := api.Group("", authMiddleware(cli.User))
	auth.POST("/seckill/do", gw.DoSeckill)
	auth.POST("/seckill/cancel", gw.CancelSeckill)
	auth.GET("/cart/list", gw.ListCart)
	auth.GET("/cart/count", gw.CountCart)
	auth.POST("/cart/add", gw.AddCartItem)
	auth.POST("/cart/update", gw.UpdateCartItem)
	auth.POST("/cart/remove", gw.RemoveCartItem)
	auth.POST("/cart/clear", gw.ClearCart)
	auth.POST("/order/pay", gw.PayOrder)
	auth.GET("/order/list", gw.ListOrders)
	auth.GET("/order/get", gw.GetOrder)

	// 管理侧：准备商品、活动并预热，压测脚本依赖这几个接口
	admin := api.Group("/admin")
	admin.POST("/product", gw.CreateProduct)
	admin.POST("/activity", gw.CreateActivity)
	admin.POST("/warmup", gw.WarmUp)
}

func parseInt64(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
