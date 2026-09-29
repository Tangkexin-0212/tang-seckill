// Package logger 统一日志初始化（复用 Kitex 自带的 klog）。
package logger

import (
	"os"
	"strings"

	"github.com/cloudwego/kitex/pkg/klog"
)

// Init 初始化全局日志输出到 stdout，并设置日志级别。
func Init(level string) {
	klog.SetOutput(os.Stdout)
	klog.SetLevel(parseLevel(level))
}

func parseLevel(level string) klog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return klog.LevelDebug
	case "info", "":
		return klog.LevelInfo
	case "warn", "warning":
		return klog.LevelWarn
	case "error":
		return klog.LevelError
	default:
		return klog.LevelInfo
	}
}
