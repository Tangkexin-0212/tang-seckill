// Package rpcx 汇总 Kitex 服务端/客户端通用配置。
package rpcx

import (
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/retry"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/transmeta"
	"github.com/cloudwego/kitex/pkg/utils"
	"github.com/cloudwego/kitex/server"
	"github.com/cloudwego/kitex/transport"

	"seckill/pkg/config"
	"seckill/pkg/registryx"
)

// ClientTimeout 客户端单次 RPC 超时。
const ClientTimeout = 3 * time.Second

// ServerOptions 构造服务端通用配置，返回 options 与监听地址。
func ServerOptions(cfg *config.Config, serviceName string, extra ...server.Option) ([]server.Option, string, error) {
	node := cfg.ServerOf(serviceName)
	r, err := registryx.NewRegistry(cfg.Etcd)
	if err != nil {
		return nil, "", err
	}

	opts := []server.Option{
		server.WithServiceAddr(utils.NewNetAddr("tcp", node.Addr())),
		server.WithRegistry(r),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: serviceName}),
		server.WithExitWaitTime(3 * time.Second),
		// 业务错误码（errno → kerrors.BizStatusError）依赖 TTHeader 透传：
		// Kitex 的 invokeHandleEndpoint 会把 BizStatusError 放进 Invocation 并返回 nil，
		// 只有在响应头里写入业务状态，客户端才能还原错误码；
		// 缺少这个 MetaHandler 时客户端只会拿到 (nil, nil)，所有业务错误码静默丢失。
		server.WithMetaHandler(transmeta.ServerTTHeaderHandler),
	}
	opts = append(opts, extra...)
	return opts, node.Addr(), nil
}

// ClientOptions 构造客户端通用配置（etcd 服务发现 + 超时 + 重试）。
func ClientOptions(cfg *config.Config) ([]client.Option, error) {
	resolver, err := registryx.NewResolver(cfg.Etcd)
	if err != nil {
		return nil, err
	}
	return []client.Option{
		client.WithResolver(resolver),
		client.WithRPCTimeout(ClientTimeout),
		client.WithConnectTimeout(time.Second),
		client.WithFailureRetry(retry.NewFailurePolicy()),
		// 与 ServerTTHeaderHandler 配对，从响应头还原业务错误码
		client.WithTransportProtocol(transport.TTHeader),
		client.WithMetaHandler(transmeta.ClientTTHeaderHandler),
	}, nil
}
