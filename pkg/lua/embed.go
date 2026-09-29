// Package lua 内嵌所有 Redis Lua 脚本，避免运行期读取文件路径带来的部署问题。
package lua

import (
	"embed"
	"fmt"
)

//go:embed *.lua
var fs embed.FS

// 脚本文件名
const (
	TokenBucket    = "token_bucket.lua"
	Seckill        = "seckill.lua"
	Release        = "release.lua"
	CartAdd        = "cart_add.lua"
	CartSeckillAdd = "cart_seckill_add.lua"
	CartSet        = "cart_set.lua"
)

// Script 返回指定脚本的内容。
func Script(name string) string {
	b, err := fs.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("lua script %s not embedded: %v", name, err))
	}
	return string(b)
}
