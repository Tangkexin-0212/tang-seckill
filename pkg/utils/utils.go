// Package utils 提供加解密、ID 生成、时间转换等通用工具。
package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// RandomHex 生成 n 字节随机数的十六进制字符串（长度 2n）。
func RandomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败属于不可恢复的环境问题，用时间戳兜底避免 panic
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// HashPassword 计算 sha256(salt+raw) 的十六进制摘要。
func HashPassword(raw, salt string) string {
	sum := sha256.Sum256([]byte(salt + raw))
	return hex.EncodeToString(sum[:])
}

// NewToken 生成登录令牌。
func NewToken() string {
	return RandomHex(16)
}

// NewOrderNo 生成订单号：SK + 毫秒时间戳 + 6 位随机。
func NewOrderNo() string {
	return "SK" + strconv.FormatInt(time.Now().UnixMilli(), 10) + RandomHex(3)
}

// NowMs 当前毫秒时间戳。
func NowMs() int64 { return time.Now().UnixMilli() }

// ToMs 把 time.Time 转为毫秒时间戳，零值返回 0。
func ToMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// FromMs 把毫秒时间戳转为 time.Time，0 返回零值。
func FromMs(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// ToMsPtr 把可空时间转为毫秒时间戳。
func ToMsPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return ToMs(*t)
}
