package main

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"gorm.io/gorm"

	"seckill/kitex_gen/user"
	"seckill/pkg/consts"
	"seckill/pkg/errno"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
	"seckill/pkg/redisx"
	"seckill/pkg/utils"
)

const tokenTTL = 24 * time.Hour

// Handler 用户服务实现。
type Handler struct {
	db  *gorm.DB
	rdb *redisx.Client
}

// NewHandler 构造用户服务处理器。
func NewHandler(db *gorm.DB, rdb *redisx.Client) *Handler {
	return &Handler{db: db, rdb: rdb}
}

// Register 注册新用户。
func (h *Handler) Register(ctx context.Context, username, password string) (int64, error) {
	if len(username) < 3 || len(password) < 6 {
		return 0, errno.ToBizError(errno.WithMsg(errno.BadRequest, "用户名至少 3 位，密码至少 6 位"))
	}

	salt := utils.RandomHex(8)
	u := &models.User{
		Username: username,
		Salt:     salt,
		Password: utils.HashPassword(password, salt),
		Nickname: username,
	}
	if err := h.db.WithContext(ctx).Create(u).Error; err != nil {
		if mysqlx.IsDupErr(err) {
			return 0, errno.ToBizError(errno.WithMsg(errno.BadRequest, "用户名已存在"))
		}
		klog.Errorf("register user %s failed: %v", username, err)
		return 0, errno.ToBizError(errno.Internal)
	}
	return int64(u.ID), nil
}

// Login 校验密码并签发 token。
func (h *Handler) Login(ctx context.Context, username, password string) (*user.LoginResp, error) {
	var u models.User
	err := h.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.WithMsg(errno.Unauthorized, "用户名或密码错误"))
		}
		klog.Errorf("query user %s failed: %v", username, err)
		return nil, errno.ToBizError(errno.Internal)
	}

	if utils.HashPassword(password, u.Salt) != u.Password {
		return nil, errno.ToBizError(errno.WithMsg(errno.Unauthorized, "用户名或密码错误"))
	}

	token := utils.NewToken()
	if err = h.rdb.Raw().Set(ctx, consts.UserTokenKey(token), u.ID, tokenTTL).Err(); err != nil {
		klog.Errorf("save token failed: %v", err)
		return nil, errno.ToBizError(errno.Internal)
	}

	return &user.LoginResp{
		Token:      token,
		Uid:        int64(u.ID),
		ExpireAtMs: utils.NowMs() + tokenTTL.Milliseconds(),
	}, nil
}

// GetUser 查询用户信息。
func (h *Handler) GetUser(ctx context.Context, uid int64) (*user.User, error) {
	var u models.User
	err := h.db.WithContext(ctx).First(&u, uint64(uid)).Error
	if err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.WithMsg(errno.BadRequest, "用户不存在"))
		}
		return nil, errno.ToBizError(errno.Internal)
	}
	return &user.User{
		Id:          int64(u.ID),
		Username:    u.Username,
		Nickname:    u.Nickname,
		CreatedAtMs: utils.ToMs(u.CreatedAt),
	}, nil
}

// VerifyToken 网关鉴权：token -> uid，失败返回 0。
func (h *Handler) VerifyToken(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, nil
	}
	val, err := h.rdb.Raw().Get(ctx, consts.UserTokenKey(token)).Int64()
	if err != nil {
		// token 不存在或已过期都属于正常鉴权失败
		return 0, nil
	}
	return val, nil
}
