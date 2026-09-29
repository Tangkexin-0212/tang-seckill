// Package registryx 封装 etcd 注册与发现。
package registryx

import (
	"fmt"
	"time"

	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/cloudwego/kitex/pkg/registry"
	etcd "github.com/kitex-contrib/registry-etcd"

	"seckill/pkg/config"
)

// NewRegistry 创建服务注册器。
func NewRegistry(conf config.EtcdConf) (registry.Registry, error) {
	r, err := etcd.NewEtcdRegistry(conf.Endpoints, etcd.WithDialTimeoutOpt(dialTimeout(conf)))
	if err != nil {
		return nil, fmt.Errorf("new etcd registry: %w", err)
	}
	return r, nil
}

// NewResolver 创建服务解析器。
func NewResolver(conf config.EtcdConf) (discovery.Resolver, error) {
	r, err := etcd.NewEtcdResolver(conf.Endpoints, etcd.WithDialTimeoutOpt(dialTimeout(conf)))
	if err != nil {
		return nil, fmt.Errorf("new etcd resolver: %w", err)
	}
	return r, nil
}

func dialTimeout(conf config.EtcdConf) time.Duration {
	if conf.DialTimeoutSeconds <= 0 {
		return 3 * time.Second
	}
	return time.Duration(conf.DialTimeoutSeconds) * time.Second
}
