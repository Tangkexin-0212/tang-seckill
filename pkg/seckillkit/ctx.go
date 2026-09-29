package seckillkit

import "context"

// snapshotCtxKey 用于在限流中间件与业务 handler 之间传递活动快照，
// 避免同一次请求重复读取 Redis。
type snapshotCtxKey struct{}

// CtxSnapshot 中间件预读的快照数据。
type CtxSnapshot struct {
	Snap   *Snapshot
	Remain int32
}

// WithSnapshot 把活动快照写入 context。
func WithSnapshot(ctx context.Context, snap *Snapshot, remain int32) context.Context {
	return context.WithValue(ctx, snapshotCtxKey{}, &CtxSnapshot{Snap: snap, Remain: remain})
}

// SnapshotFrom 从 context 取出活动快照。
func SnapshotFrom(ctx context.Context) (*Snapshot, int32, bool) {
	v, ok := ctx.Value(snapshotCtxKey{}).(*CtxSnapshot)
	if !ok || v == nil || v.Snap == nil {
		return nil, 0, false
	}
	return v.Snap, v.Remain, true
}
