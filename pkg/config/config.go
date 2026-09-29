// Package config 负责加载与访问全局 YAML 配置。
package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// MySQLConf MySQL 连接配置。
type MySQLConf struct {
	DSN                    string `yaml:"dsn"`
	MaxIdleConns           int    `yaml:"maxIdleConns"`
	MaxOpenConns           int    `yaml:"maxOpenConns"`
	ConnMaxLifetimeSeconds int    `yaml:"connMaxLifetimeSeconds"`
	SlowThresholdMs        int    `yaml:"slowThresholdMs"`
}

// DSNFor 返回指定库名的连接串。
func (m MySQLConf) DSNFor(dbName string) string {
	return fmt.Sprintf(m.DSN, dbName)
}

// RedisConf Redis 连接配置。
type RedisConf struct {
	Addr           string `yaml:"addr"`
	Password       string `yaml:"password"`
	DB             int    `yaml:"db"`
	PoolSize       int    `yaml:"poolSize"`
	MinIdleConns   int    `yaml:"minIdleConns"`
	DialTimeoutMs  int    `yaml:"dialTimeoutMs"`
	ReadTimeoutMs  int    `yaml:"readTimeoutMs"`
	WriteTimeoutMs int    `yaml:"writeTimeoutMs"`
}

// EtcdConf 注册中心配置。
type EtcdConf struct {
	Endpoints          []string `yaml:"endpoints"`
	DialTimeoutSeconds int      `yaml:"dialTimeoutSeconds"`
}

// ServerNode 单个服务的监听地址。
type ServerNode struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// Addr 返回 host:port。
func (s ServerNode) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// SeckillConf 秒杀业务参数。
type SeckillConf struct {
	SnapshotTTLBufferSeconds int `yaml:"snapshotTTLBufferSeconds"`
	CartMaxQty               int `yaml:"cartMaxQty"`
}

// CartConf 购物车参数。
type CartConf struct {
	MaxPerItem int `yaml:"maxPerItem"`
	TTLSeconds int `yaml:"ttlSeconds"`
}

// OrderConf 订单业务参数。
type OrderConf struct {
	PayTimeoutSeconds   int `yaml:"payTimeoutSeconds"`
	DelayScanIntervalMs int `yaml:"delayScanIntervalMs"`
}

// BucketConf 单个令牌桶参数。
type BucketConf struct {
	Capacity         float64 `yaml:"capacity"`
	RatePerSecond    float64 `yaml:"ratePerSecond"`
	CapacityFactor   float64 `yaml:"capacityFactor"`
	RateFactor       float64 `yaml:"rateFactor"`
	FallbackCapacity float64 `yaml:"fallbackCapacity"`
	FallbackRate     float64 `yaml:"fallbackRate"`
}

// ActivityBucketConf 商品维度令牌桶配置。
type ActivityBucketConf struct {
	Enabled          bool    `yaml:"enabled"`
	CapacityFactor   float64 `yaml:"capacityFactor"`
	RateFactor       float64 `yaml:"rateFactor"`
	FallbackCapacity float64 `yaml:"fallbackCapacity"`
	FallbackRate     float64 `yaml:"fallbackRate"`
}

// UserBucketConf 用户维度令牌桶配置。
type UserBucketConf struct {
	Enabled       bool    `yaml:"enabled"`
	Capacity      float64 `yaml:"capacity"`
	RatePerSecond float64 `yaml:"ratePerSecond"`
}

// RateLimitConf 限流配置。
type RateLimitConf struct {
	Global   BucketConf         `yaml:"global"`
	Activity ActivityBucketConf `yaml:"activity"`
	User     UserBucketConf     `yaml:"user"`
	FailOpen bool               `yaml:"failOpen"`
}

// CronConf 定时任务配置。
type CronConf struct {
	SyncSpec      string `yaml:"syncSpec"`
	ReconcileSpec string `yaml:"reconcileSpec"`
	WarmUpOnStart bool   `yaml:"warmUpOnStart"`
}

// WorkerConf 消费者配置。
type WorkerConf struct {
	ConsumerName string `yaml:"consumerName"`
	BatchSize    int64  `yaml:"batchSize"`
	BlockMs      int64  `yaml:"blockMs"`
	MaxRetry     int    `yaml:"maxRetry"`
}

// Config 全局配置根。
type Config struct {
	Env       string                `yaml:"env"`
	MySQL     MySQLConf             `yaml:"mysql"`
	Redis     RedisConf             `yaml:"redis"`
	Etcd      EtcdConf              `yaml:"etcd"`
	Server    map[string]ServerNode `yaml:"server"`
	Seckill   SeckillConf           `yaml:"seckill"`
	Cart      CartConf              `yaml:"cart"`
	Order     OrderConf             `yaml:"order"`
	RateLimit RateLimitConf         `yaml:"ratelimit"`
	Cron      CronConf              `yaml:"cron"`
	Worker    WorkerConf            `yaml:"worker"`
}

var (
	once   sync.Once
	global *Config
)

// DefaultPath 默认配置文件路径。
const DefaultPath = "config.yaml"

// Load 读取 YAML 配置文件并填充默认值。
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err = yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	c.applyDefaults()
	return &c, nil
}

// MustLoad 读取配置失败时 panic（仅用于进程启动阶段）。
func MustLoad(path string) *Config {
	c, err := Load(path)
	if err != nil {
		panic(err)
	}
	once.Do(func() { global = c })
	return c
}

// Get 返回已加载的全局配置。
func Get() *Config {
	if global == nil {
		return MustLoad("")
	}
	return global
}

// ServerOf 返回指定服务的监听配置。
func (c *Config) ServerOf(name string) ServerNode {
	if n, ok := c.Server[name]; ok {
		return n
	}
	return ServerNode{Host: "127.0.0.1", Port: 0}
}

// IsDev 是否开发环境。
func (c *Config) IsDev() bool { return c.Env == "dev" }

func (c *Config) applyDefaults() {
	if c.MySQL.MaxIdleConns == 0 {
		c.MySQL.MaxIdleConns = 10
	}
	if c.MySQL.MaxOpenConns == 0 {
		c.MySQL.MaxOpenConns = 50
	}
	if c.MySQL.ConnMaxLifetimeSeconds == 0 {
		c.MySQL.ConnMaxLifetimeSeconds = 3600
	}
	if c.MySQL.SlowThresholdMs == 0 {
		c.MySQL.SlowThresholdMs = 200
	}
	if c.Redis.PoolSize == 0 {
		c.Redis.PoolSize = 100
	}
	if c.Redis.DialTimeoutMs == 0 {
		c.Redis.DialTimeoutMs = 1000
	}
	if c.Redis.ReadTimeoutMs == 0 {
		c.Redis.ReadTimeoutMs = 1000
	}
	if c.Redis.WriteTimeoutMs == 0 {
		c.Redis.WriteTimeoutMs = 1000
	}
	if c.Etcd.DialTimeoutSeconds == 0 {
		c.Etcd.DialTimeoutSeconds = 3
	}
	if c.Seckill.SnapshotTTLBufferSeconds == 0 {
		c.Seckill.SnapshotTTLBufferSeconds = 60
	}
	if c.Seckill.CartMaxQty == 0 {
		c.Seckill.CartMaxQty = 1
	}
	if c.Cart.MaxPerItem == 0 {
		c.Cart.MaxPerItem = 99
	}
	if c.Cart.TTLSeconds == 0 {
		c.Cart.TTLSeconds = 604800
	}
	if c.Order.PayTimeoutSeconds == 0 {
		c.Order.PayTimeoutSeconds = 900
	}
	if c.Order.DelayScanIntervalMs == 0 {
		c.Order.DelayScanIntervalMs = 200
	}
	if c.RateLimit.Global.Capacity == 0 {
		c.RateLimit.Global.Capacity = 5000
		c.RateLimit.Global.RatePerSecond = 3000
	}
	if c.RateLimit.Activity.CapacityFactor == 0 {
		c.RateLimit.Activity.CapacityFactor = 1.5
		c.RateLimit.Activity.RateFactor = 3
		c.RateLimit.Activity.FallbackCapacity = 200
		c.RateLimit.Activity.FallbackRate = 100
	}
	if c.RateLimit.User.Capacity == 0 {
		c.RateLimit.User.Capacity = 5
		c.RateLimit.User.RatePerSecond = 1
	}
	if c.Cron.SyncSpec == "" {
		c.Cron.SyncSpec = "*/30 * * * * *"
	}
	if c.Cron.ReconcileSpec == "" {
		c.Cron.ReconcileSpec = "0 * * * * *"
	}
	if c.Worker.ConsumerName == "" {
		c.Worker.ConsumerName = "worker-1"
	}
	if c.Worker.BatchSize == 0 {
		c.Worker.BatchSize = 100
	}
	if c.Worker.BlockMs == 0 {
		c.Worker.BlockMs = 2000
	}
	if c.Worker.MaxRetry == 0 {
		c.Worker.MaxRetry = 3
	}
}
