// 购物车服务：数据全部存放在 Redis Hash 中。
//
//	sk:cart:{uid}       field=productId  value=qty
//	sk:cart:snap:{uid}  field=productId  value=商品快照 JSON
package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/cart"
	"seckill/kitex_gen/product/productservice"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/errno"
	"seckill/pkg/redisx"
	"seckill/pkg/utils"
)

// cartSnap 购物车中冗余的商品快照，避免每次读购物车都回源商品服务。
type cartSnap struct {
	ProductName   string `json:"productName"`
	Cover         string `json:"cover"`
	OriginalPrice int64  `json:"originalPrice"`
	Price         int64  `json:"price"`
	ActivityID    int64  `json:"activityId"`
}

// Handler 购物车服务实现。
type Handler struct {
	rdb     *redisx.Client
	product productservice.Client
	cfg     *config.Config
}

// NewHandler 构造购物车处理器。
func NewHandler(rdb *redisx.Client, product productservice.Client, cfg *config.Config) *Handler {
	return &Handler{rdb: rdb, product: product, cfg: cfg}
}

func (h *Handler) ttl() time.Duration {
	return time.Duration(h.cfg.Cart.TTLSeconds) * time.Second
}

// AddItem 普通加购：已存在则累加，并校验单商品上限。
func (h *Handler) AddItem(ctx context.Context, userID, productID int64, qty int32) (*cart.CartItem, error) {
	if userID <= 0 || productID <= 0 || qty <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	code, cur, err := h.rdb.CartAdd(ctx, consts.CartKey(userID), productID, qty, int32(h.cfg.Cart.MaxPerItem))
	if err != nil {
		klog.Errorf("cart add failed uid=%d pid=%d: %v", userID, productID, err)
		return nil, errno.ToBizError(errno.Internal)
	}
	if code == redisx.CartOverLimit {
		return nil, errno.ToBizError(errno.CartLimit)
	}
	if code == redisx.CartBadArg {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	snap := h.loadSnap(ctx, userID, productID)
	if snap.ProductName == "" {
		if snap = h.fetchProductSnap(ctx, productID); snap.ProductName != "" {
			h.saveSnap(ctx, userID, productID, snap)
		}
	}
	return buildItem(userID, productID, cur, snap), nil
}

// SeckillAdd 秒杀加购：幂等，无论调用多少次 qty 恒为 1。
func (h *Handler) SeckillAdd(ctx context.Context, userID, productID, activityID, price int64,
	productName, cover string) (*cart.CartItem, error) {
	if userID <= 0 || productID <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	qty := int32(h.cfg.Seckill.CartMaxQty)
	if qty <= 0 {
		qty = 1
	}

	// 关键：HSETNX 语义，重复调用不会把 qty 累加成 2/3/4
	_, cur, err := h.rdb.CartSeckillAdd(ctx, consts.CartKey(userID), productID, qty)
	if err != nil {
		klog.Errorf("cart seckill add failed uid=%d pid=%d: %v", userID, productID, err)
		return nil, errno.ToBizError(errno.Internal)
	}

	snap := cartSnap{
		ProductName: productName,
		Cover:       cover,
		Price:       price,
		ActivityID:  activityID,
	}
	if snap.ProductName == "" {
		if fetched := h.loadSnap(ctx, userID, productID); fetched.ProductName != "" {
			snap.ProductName = fetched.ProductName
			snap.Cover = fetched.Cover
			snap.OriginalPrice = fetched.OriginalPrice
		}
	}
	h.saveSnap(ctx, userID, productID, snap)

	return buildItem(userID, productID, cur, snap), nil
}

// UpdateQty 修改数量，qty<=0 视为删除。
func (h *Handler) UpdateQty(ctx context.Context, userID, productID int64, qty int32) (*cart.CartItem, error) {
	if userID <= 0 || productID <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	code, cur, err := h.rdb.CartSet(ctx, consts.CartKey(userID), productID, qty, int32(h.cfg.Cart.MaxPerItem))
	if err != nil {
		klog.Errorf("cart update failed uid=%d pid=%d: %v", userID, productID, err)
		return nil, errno.ToBizError(errno.Internal)
	}
	switch code {
	case redisx.CartOverLimit:
		return nil, errno.ToBizError(errno.CartLimit)
	case redisx.CartBadArg:
		return nil, errno.ToBizError(errno.BadRequest)
	case redisx.CartDeleted:
		h.removeSnap(ctx, userID, productID)
		return &cart.CartItem{UserId: userID, ProductId: productID, Qty: 0}, nil
	default:
		return buildItem(userID, productID, cur, h.loadSnap(ctx, userID, productID)), nil
	}
}

// RemoveItem 删除购物车条目。
func (h *Handler) RemoveItem(ctx context.Context, userID, productID int64) (bool, error) {
	if userID <= 0 || productID <= 0 {
		return false, errno.ToBizError(errno.BadRequest)
	}
	field := strconv.FormatInt(productID, 10)
	pipe := h.rdb.Raw().Pipeline()
	pipe.HDel(ctx, consts.CartKey(userID), field)
	pipe.HDel(ctx, consts.CartSnapKey(userID), field)
	if _, err := pipe.Exec(ctx); err != nil {
		klog.Errorf("cart remove failed uid=%d pid=%d: %v", userID, productID, err)
		return false, errno.ToBizError(errno.Internal)
	}
	return true, nil
}

// ClearCart 清空购物车。
func (h *Handler) ClearCart(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, errno.ToBizError(errno.BadRequest)
	}
	if err := h.rdb.Raw().Del(ctx, consts.CartKey(userID), consts.CartSnapKey(userID)).Err(); err != nil {
		klog.Errorf("cart clear failed uid=%d: %v", userID, err)
		return false, errno.ToBizError(errno.Internal)
	}
	return true, nil
}

// ListCart 列出购物车。
func (h *Handler) ListCart(ctx context.Context, userID int64) ([]*cart.CartItem, error) {
	if userID <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	raw, err := h.rdb.Raw().HGetAll(ctx, consts.CartKey(userID)).Result()
	if err != nil {
		klog.Errorf("cart list failed uid=%d: %v", userID, err)
		return nil, errno.ToBizError(errno.Internal)
	}
	if len(raw) == 0 {
		return []*cart.CartItem{}, nil
	}

	snaps, err := h.rdb.Raw().HGetAll(ctx, consts.CartSnapKey(userID)).Result()
	if err != nil {
		klog.Warnf("cart snap list failed uid=%d: %v", userID, err)
	}

	items := make([]*cart.CartItem, 0, len(raw))
	for field, val := range raw {
		pid, perr := strconv.ParseInt(field, 10, 64)
		if perr != nil {
			continue
		}
		qty, qerr := strconv.Atoi(val)
		if qerr != nil {
			continue
		}
		var snap cartSnap
		if s, ok := snaps[field]; ok {
			_ = json.Unmarshal([]byte(s), &snap)
		}
		items = append(items, buildItem(userID, pid, int32(qty), snap))
	}
	return items, nil
}

// CountItems 统计购物车条目数。
func (h *Handler) CountItems(ctx context.Context, userID int64) (int32, error) {
	if userID <= 0 {
		return 0, errno.ToBizError(errno.BadRequest)
	}
	n, err := h.rdb.Raw().HLen(ctx, consts.CartKey(userID)).Result()
	if err != nil {
		return 0, errno.ToBizError(errno.Internal)
	}
	return int32(n), nil
}

// ------------------------------------------------------------
// 内部方法
// ------------------------------------------------------------

func (h *Handler) loadSnap(ctx context.Context, userID, productID int64) cartSnap {
	val, err := h.rdb.Raw().HGet(ctx, consts.CartSnapKey(userID), strconv.FormatInt(productID, 10)).Result()
	if err != nil {
		return cartSnap{}
	}
	var snap cartSnap
	if err = json.Unmarshal([]byte(val), &snap); err != nil {
		return cartSnap{}
	}
	return snap
}

func (h *Handler) saveSnap(ctx context.Context, userID, productID int64, snap cartSnap) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	field := strconv.FormatInt(productID, 10)
	pipe := h.rdb.Raw().Pipeline()
	pipe.HSet(ctx, consts.CartSnapKey(userID), field, raw)
	pipe.Expire(ctx, consts.CartSnapKey(userID), h.ttl())
	pipe.Expire(ctx, consts.CartKey(userID), h.ttl())
	if _, err = pipe.Exec(ctx); err != nil {
		klog.Warnf("save cart snap failed uid=%d pid=%d: %v", userID, productID, err)
	}
}

func (h *Handler) removeSnap(ctx context.Context, userID, productID int64) {
	_ = h.rdb.Raw().HDel(ctx, consts.CartSnapKey(userID), strconv.FormatInt(productID, 10)).Err()
}

// fetchProductSnap 回源商品服务补全快照（best-effort，失败不影响加购）。
func (h *Handler) fetchProductSnap(ctx context.Context, productID int64) cartSnap {
	if h.product == nil {
		return cartSnap{}
	}
	p, err := h.product.GetProduct(ctx, productID)
	if err != nil || p == nil {
		klog.Warnf("fetch product %d for cart snap failed: %v", productID, err)
		return cartSnap{}
	}
	return cartSnap{ProductName: p.Name, Cover: p.Cover, OriginalPrice: p.Price, Price: p.Price}
}

func buildItem(userID, productID int64, qty int32, snap cartSnap) *cart.CartItem {
	original := snap.OriginalPrice
	if original <= 0 {
		original = snap.Price
	}
	return &cart.CartItem{
		UserId:        userID,
		ProductId:     productID,
		Qty:           qty,
		ProductName:   snap.ProductName,
		Cover:         snap.Cover,
		OriginalPrice: original,
		Price:         snap.Price,
		ActivityId:    snap.ActivityID,
	}
}

// 保持 utils 引用（订单号等能力由上层使用）。
var _ = utils.NowMs
