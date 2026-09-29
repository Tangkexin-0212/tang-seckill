package main

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/pkg/rpcinfo"

	"seckill/pkg/errno"
)

// Middleware 服务端统一中间件：panic 兜底 + 访问日志 + 错误归一化。
//
// 秒杀是核心链路，任何一处 panic 都会让整个服务实例不可用，
// 因此这里必须 recover 并把异常转成业务错误返回，而不是让连接直接断开。
func Middleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp interface{}) (err error) {
			start := time.Now()

			defer func() {
				if r := recover(); r != nil {
					klog.Errorf("panic recovered, method=%s err=%v\n%s",
						methodOf(ctx), r, debug.Stack())
					err = errno.ToBizError(errno.WithMsg(errno.Internal, "服务内部异常"))
				}
			}()

			callErr := next(ctx, req, resp)

			cost := time.Since(start)
			info := rpcinfo.GetRPCInfo(ctx)
			from := "-"
			if info != nil && info.From() != nil {
				from = info.From().ServiceName()
			}

			if callErr == nil {
				klog.Infof("seckill done method=%s from=%s cost=%s", methodOf(ctx), from, cost)
				return nil
			}

			// 归一化成业务错误再返回：调用方拿到稳定的错误码，
			// 同时避免把内部堆栈/DB 语句等敏感信息透出去。
			e := errno.From(callErr)
			klog.Warnf("seckill failed method=%s from=%s cost=%s code=%d msg=%s",
				methodOf(ctx), from, cost, e.Code, e.Msg)
			return errno.ToBizError(e)
		}
	}
}

func methodOf(ctx context.Context) string {
	if info := rpcinfo.GetRPCInfo(ctx); info != nil && info.To() != nil {
		return info.To().Method()
	}
	return "unknown"
}
