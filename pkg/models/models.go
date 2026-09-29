// Package models 定义各服务的 GORM 数据模型。
package models

import "time"

// ------------------------------------------------------------
// user_db
// ------------------------------------------------------------

// User 用户表。
type User struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Username  string    `gorm:"column:username;size:64;not null;uniqueIndex:uk_username"`
	Password  string    `gorm:"column:password;size:128;not null"`
	Salt      string    `gorm:"column:salt;size:32;not null"`
	Nickname  string    `gorm:"column:nickname;size:64;not null;default:''"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名。
func (User) TableName() string { return "user" }

// ------------------------------------------------------------
// product_db
// ------------------------------------------------------------

// Product 商品表。
type Product struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string    `gorm:"column:name;size:128;not null"`
	Price       int64     `gorm:"column:price;not null"`
	Cover       string    `gorm:"column:cover;size:255;not null;default:''"`
	Description string    `gorm:"column:description;type:text"`
	Status      int32     `gorm:"column:status;not null;default:1"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名。
func (Product) TableName() string { return "product" }

// 活动状态
const (
	ActivityNotStarted int32 = 0
	ActivityRunning    int32 = 1
	ActivityFinished   int32 = 2
)

// SeckillActivity 秒杀活动表。
type SeckillActivity struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ProductID    uint64    `gorm:"column:product_id;not null;index:idx_product"`
	SeckillPrice int64     `gorm:"column:seckill_price;not null"`
	TotalStock   int32     `gorm:"column:total_stock;not null"`
	Stock        int32     `gorm:"column:stock;not null"`
	StartTime    time.Time `gorm:"column:start_time;not null"`
	EndTime      time.Time `gorm:"column:end_time;not null"`
	Status       int32     `gorm:"column:status;not null;default:0"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名。
func (SeckillActivity) TableName() string { return "seckill_activity" }

// ------------------------------------------------------------
// stock_db
// ------------------------------------------------------------

// Stock 商品库存表。
type Stock struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ProductID uint64    `gorm:"column:product_id;not null;uniqueIndex:uk_product"`
	Total     int32     `gorm:"column:total;not null"`
	Available int32     `gorm:"column:available;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名。
func (Stock) TableName() string { return "stock" }

// 库存流水类型
const (
	StockFlowDeduct   int8 = 1
	StockFlowRollback int8 = 2
)

// StockFlow 库存流水表，uk(order_no, type) 保证扣减/回补各自只发生一次。
type StockFlow struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ActivityID uint64    `gorm:"column:activity_id;not null"`
	UserID     uint64    `gorm:"column:user_id;not null"`
	OrderNo    string    `gorm:"column:order_no;size:64;not null;uniqueIndex:uk_order_type"`
	Delta      int32     `gorm:"column:delta;not null"`
	Type       int8      `gorm:"column:type;not null;uniqueIndex:uk_order_type"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName 指定表名。
func (StockFlow) TableName() string { return "stock_flow" }

// ------------------------------------------------------------
// order_db
// ------------------------------------------------------------

// 订单状态
const (
	OrderPending   int32 = 0
	OrderPaid      int32 = 1
	OrderCancelled int32 = 2
)

// Order 订单表，uk(user_id, activity_id) 是"一人一件"的 DB 兜底。
type Order struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	OrderNo     string     `gorm:"column:order_no;size:64;not null;uniqueIndex:uk_order_no"`
	UserID      uint64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_activity"`
	ActivityID  uint64     `gorm:"column:activity_id;not null;uniqueIndex:uk_user_activity"`
	ProductID   uint64     `gorm:"column:product_id;not null"`
	ProductName string     `gorm:"column:product_name;size:128;not null;default:''"`
	Amount      int64      `gorm:"column:amount;not null"`
	Status      int32      `gorm:"column:status;not null;default:0"`
	ExpireAt    time.Time  `gorm:"column:expire_at;not null"`
	PaidAt      *time.Time `gorm:"column:paid_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

// TableName 指定表名。
func (Order) TableName() string { return "order" }

// MQConsumeLog 消费者幂等表。
type MQConsumeLog struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	MsgID     string    `gorm:"column:msg_id;size:64;not null;uniqueIndex:uk_msg_consumer"`
	Consumer  string    `gorm:"column:consumer;size:64;not null;uniqueIndex:uk_msg_consumer"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName 指定表名。
func (MQConsumeLog) TableName() string { return "mq_consume_log" }
