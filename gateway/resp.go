// Package main 网关层：统一响应体与错误码映射。
package main

import (
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"

	"seckill/pkg/errno"
)

// Resp 统一响应体。约定：HTTP 状态码恒为 200，业务结果一律看 code 字段。
// 这样前端与压测脚本只需解析一个维度，不会出现"HTTP 200 但 body 是错误"的歧义。
type Resp struct {
	Code int32       `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// ok 返回成功响应。
func ok(c *app.RequestContext, data interface{}) {
	c.JSON(http.StatusOK, Resp{Code: errno.CodeOK, Msg: "success", Data: data})
}

// fail 返回失败响应，把下游的业务错误码原样透传给调用方。
func fail(c *app.RequestContext, err error) {
	e := errno.From(err)
	c.JSON(http.StatusOK, Resp{Code: e.Code, Msg: e.Msg})
}

// badRequest 参数错误快捷方法。
func badRequest(c *app.RequestContext, msg string) {
	e := errno.BadRequest
	if msg != "" {
		e = errno.WithMsg(errno.BadRequest, msg)
	}
	fail(c, e)
}
