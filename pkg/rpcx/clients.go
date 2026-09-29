package rpcx

import (
	"fmt"

	"seckill/kitex_gen/cart/cartservice"
	"seckill/kitex_gen/order/orderservice"
	"seckill/kitex_gen/product/productservice"
	"seckill/kitex_gen/seckill/seckillservice"
	"seckill/kitex_gen/stock/stockservice"
	"seckill/kitex_gen/user/userservice"
	"seckill/pkg/config"
	"seckill/pkg/consts"
)

// Clients 聚合所有下游服务客户端。
// etcd 解析是惰性的，因此即使某些服务尚未启动，构造也不会失败。
type Clients struct {
	User    userservice.Client
	Product productservice.Client
	Stock   stockservice.Client
	Cart    cartservice.Client
	Order   orderservice.Client
	Seckill seckillservice.Client
}

// NewClients 一次性构造全部客户端。
func NewClients(cfg *config.Config) (*Clients, error) {
	opts, err := ClientOptions(cfg)
	if err != nil {
		return nil, err
	}

	users, err := userservice.NewClient(consts.ServiceUser, opts...)
	if err != nil {
		return nil, fmt.Errorf("new user client: %w", err)
	}
	products, err := productservice.NewClient(consts.ServiceProduct, opts...)
	if err != nil {
		return nil, fmt.Errorf("new product client: %w", err)
	}
	stocks, err := stockservice.NewClient(consts.ServiceStock, opts...)
	if err != nil {
		return nil, fmt.Errorf("new stock client: %w", err)
	}
	carts, err := cartservice.NewClient(consts.ServiceCart, opts...)
	if err != nil {
		return nil, fmt.Errorf("new cart client: %w", err)
	}
	orders, err := orderservice.NewClient(consts.ServiceOrder, opts...)
	if err != nil {
		return nil, fmt.Errorf("new order client: %w", err)
	}
	seckills, err := seckillservice.NewClient(consts.ServiceSeckill, opts...)
	if err != nil {
		return nil, fmt.Errorf("new seckill client: %w", err)
	}

	return &Clients{
		User:    users,
		Product: products,
		Stock:   stocks,
		Cart:    carts,
		Order:   orders,
		Seckill: seckills,
	}, nil
}
