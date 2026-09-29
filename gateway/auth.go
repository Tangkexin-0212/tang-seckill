package main

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/user/userservice"
	"seckill/pkg/errno"
)

// uidKey 登录用户在 RequestContext 中的键。
const uidKey = "uid"

// authMiddleware 校验 Authorization 头中的 token，并把 uid 注入请求上下文。
// 鉴权下沉到网关，秒杀/购物车等下游服务只信任网关传来的 userId。
func authMiddleware(user userservice.Client) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		token := extractToken(c)
		if token == "" {
			fail(c, errno.Unauthorized)
			c.Abort()
			return
		}

		uid, err := user.VerifyToken(ctx, token)
		if err != nil {
			klog.Warnf("verify token failed: %v", err)
			fail(c, errno.Internal)
			c.Abort()
			return
		}
		if uid <= 0 {
			fail(c, errno.Unauthorized)
			c.Abort()
			return
		}

		c.Set(uidKey, uid)
		c.Next(ctx)
	}
}

// extractToken 支持 "Authorization: Bearer xxx" 与 "Authorization: xxx" 两种写法。
func extractToken(c *app.RequestContext) string {
	raw := strings.TrimSpace(string(c.GetHeader("Authorization")))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("token"))
	}
	if raw == "" {
		return ""
	}
	if len(raw) > 7 && strings.EqualFold(raw[:7], "Bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return raw
}

// currentUID 取出当前登录用户 ID，未登录返回 0。
func currentUID(c *app.RequestContext) int64 {
	return c.GetInt64(uidKey)
}
