// Package errno 定义业务错误码，并提供与 Kitex 业务错误的互转。
package errno

import (
	"errors"

	"github.com/cloudwego/kitex/pkg/kerrors"
)

// 错误码定义
const (
	CodeOK              int32 = 0
	CodeRateLimited     int32 = 10001
	CodeAlreadyBought   int32 = 10002
	CodeSoldOut         int32 = 10003
	CodeActivityInvalid int32 = 10004
	CodeUnauthorized    int32 = 10005
	CodeBadRequest      int32 = 10006
	CodeOrderNotFound   int32 = 10007
	CodeOrderState      int32 = 10008
	CodeCartLimit       int32 = 10009
	CodeDuplicateReq    int32 = 10010
	CodeInternal        int32 = 20001
	CodeRollbackFailed  int32 = 20002
)

// Err 业务错误。
type Err struct {
	Code int32
	Msg  string
}

// Error 实现 error 接口。
func (e *Err) Error() string { return e.Msg }

// New 构造业务错误。
func New(code int32, msg string) *Err { return &Err{Code: code, Msg: msg} }

// 预定义错误
var (
	OK              = &Err{Code: CodeOK, Msg: "success"}
	RateLimited     = &Err{Code: CodeRateLimited, Msg: "请求过于频繁，请稍后再试"}
	AlreadyBought   = &Err{Code: CodeAlreadyBought, Msg: "您已参与过该活动，每人限购 1 件"}
	SoldOut         = &Err{Code: CodeSoldOut, Msg: "商品已售罄"}
	ActivityInvalid = &Err{Code: CodeActivityInvalid, Msg: "活动不存在或不在活动时间内"}
	Unauthorized    = &Err{Code: CodeUnauthorized, Msg: "未登录或登录已过期"}
	BadRequest      = &Err{Code: CodeBadRequest, Msg: "参数错误"}
	OrderNotFound   = &Err{Code: CodeOrderNotFound, Msg: "订单不存在"}
	OrderState      = &Err{Code: CodeOrderState, Msg: "订单状态不允许该操作"}
	CartLimit       = &Err{Code: CodeCartLimit, Msg: "购物车商品数量超出上限"}
	DuplicateReq    = &Err{Code: CodeDuplicateReq, Msg: "重复请求"}
	Internal        = &Err{Code: CodeInternal, Msg: "系统繁忙，请稍后再试"}
	RollbackFailed  = &Err{Code: CodeRollbackFailed, Msg: "库存回补失败"}
)

// WithMsg 基于已有错误码构造带自定义描述的错误。
func WithMsg(base *Err, msg string) *Err {
	return &Err{Code: base.Code, Msg: msg}
}

// From 把任意 error 归一化成 *Err。
func From(err error) *Err {
	if err == nil {
		return OK
	}
	var e *Err
	if errors.As(err, &e) {
		return e
	}
	if biz, ok := kerrors.FromBizStatusError(err); ok {
		return &Err{Code: biz.BizStatusCode(), Msg: biz.BizMessage()}
	}
	return WithMsg(Internal, err.Error())
}

// ToBizError 把业务错误转成 Kitex 业务错误，便于跨进程透传错误码。
func ToBizError(e *Err) error {
	if e == nil || e.Code == CodeOK {
		return nil
	}
	return kerrors.NewBizStatusError(e.Code, e.Msg)
}
