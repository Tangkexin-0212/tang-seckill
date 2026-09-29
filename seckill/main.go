// 秒杀服务进程入口。
//
// 该服务不直连 MySQL：库存与幂等状态全在 Redis，订单落在 order 服务，
// 购物车落在 cart 服务，因此 bootstrap.Init 传空库名跳过 DB 初始化。
package main

import (
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/server"

	"seckill/kitex_gen/cart/cartservice"
	"seckill/kitex_gen/order/orderservice"
	"seckill/kitex_gen/product/productservice"
	"seckill/kitex_gen/seckill/seckillservice"
	"seckill/pkg/bootstrap"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/rpcx"
	"seckill/pkg/seckillkit"
)

func main() {
	app := bootstrap.Init("")
	defer app.Close()

	product, cart, order, err := newClients(app.Cfg)
	if err != nil {
		klog.Fatalf("build downstream clients failed: %v", err)
	}

	sync := seckillkit.NewSyncer(app.Redis, app.Cfg)
	rel := seckillkit.NewReleaser(app.Redis, app.Cfg, order, cart)
	lim := NewLimiter(app.Redis, app.Cfg)

	handler := NewHandler(app.Cfg, app.Redis, sync, rel, lim, product, cart, order)

	opts, addr, err := rpcx.ServerOptions(app.Cfg, consts.ServiceSeckill, server.WithMiddleware(Middleware()))
	if err != nil {
		klog.Fatalf("build server options failed: %v", err)
	}

	srv := seckillservice.NewServer(handler, opts...)
	klog.Infof("seckill service listening on %s", addr)
	if err = srv.Run(); err != nil {
		klog.Fatalf("seckill service exit: %v", err)
	}
}

// newClients 只构造秒杀链路真正依赖的三个下游客户端。
func newClients(cfg *config.Config) (productservice.Client, cartservice.Client, orderservice.Client, error) {
	var (
		product productservice.Client
		cart    cartservice.Client
		order   orderservice.Client
	)

	opts, err := rpcx.ClientOptions(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	if product, err = productservice.NewClient(consts.ServiceProduct, opts...); err != nil {
		return nil, nil, nil, err
	}
	if cart, err = cartservice.NewClient(consts.ServiceCart, opts...); err != nil {
		return nil, nil, nil, err
	}
	if order, err = orderservice.NewClient(consts.ServiceOrder, opts...); err != nil {
		return nil, nil, nil, err
	}
	return product, cart, order, nil
}

// 保持 client 包引用（ClientOptions 的返回类型来自该包）。
var _ []client.Option
