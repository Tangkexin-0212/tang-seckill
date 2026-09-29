// Package mysqlx 提供 GORM 初始化工具。
package mysqlx

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"seckill/pkg/config"
)

// Open 连接指定库并完成连接池配置。
func Open(conf config.MySQLConf, dbName string) (*gorm.DB, error) {
	logCfg := gormlogger.Config{
		SlowThreshold:             time.Duration(conf.SlowThresholdMs) * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	}

	db, err := gorm.Open(mysql.Open(conf.DSNFor(dbName)), &gorm.Config{
		Logger:                                   gormlogger.New(klogWriter{}, logCfg),
		DisableForeignKeyConstraintWhenMigrating: true,
		SkipDefaultTransaction:                   true,
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql %s: %w", dbName, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB of %s: %w", dbName, err)
	}
	sqlDB.SetMaxIdleConns(conf.MaxIdleConns)
	sqlDB.SetMaxOpenConns(conf.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(conf.ConnMaxLifetimeSeconds) * time.Second)

	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql %s: %w", dbName, err)
	}
	return db, nil
}

// IsDupErr 判断是否唯一键冲突（MySQL 1062）。
func IsDupErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1062") || strings.Contains(msg, "Duplicate entry")
}

// IsRecordNotFound 判断是否记录不存在。
func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// klogWriter 把 GORM 日志桥接到 Kitex klog。
type klogWriter struct{}

// Printf 实现 gormlogger.Writer。
func (klogWriter) Printf(format string, args ...interface{}) {
	klog.Warnf(format, args...)
}
