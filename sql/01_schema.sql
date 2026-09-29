-- ============================================================
-- 秒杀系统 · 建库建表 + 演示数据
-- 由 docker-compose 挂载到 /docker-entrypoint-initdb.d 自动执行
-- ============================================================

CREATE DATABASE IF NOT EXISTS `user_db` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS `product_db` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS `stock_db` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS `cart_db` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS `order_db` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- ------------------------------------------------------------
-- user_db
-- ------------------------------------------------------------
USE `user_db`;

CREATE TABLE IF NOT EXISTS `user` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `username`   VARCHAR(64)  NOT NULL,
  `password`   VARCHAR(128) NOT NULL COMMENT 'sha256(salt + raw)',
  `salt`       VARCHAR(32)  NOT NULL,
  `nickname`   VARCHAR(64)  NOT NULL DEFAULT '',
  `created_at` DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ------------------------------------------------------------
-- product_db
-- ------------------------------------------------------------
USE `product_db`;

CREATE TABLE IF NOT EXISTS `product` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name`        VARCHAR(128) NOT NULL,
  `price`       BIGINT       NOT NULL COMMENT '单位: 分',
  `cover`       VARCHAR(255) NOT NULL DEFAULT '',
  `description` TEXT,
  `status`      TINYINT      NOT NULL DEFAULT 1 COMMENT '1上架 0下架',
  `created_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `seckill_activity` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `product_id`    BIGINT UNSIGNED NOT NULL,
  `seckill_price` BIGINT          NOT NULL COMMENT '单位: 分',
  `total_stock`   INT             NOT NULL,
  `stock`         INT             NOT NULL,
  `start_time`    DATETIME(3)     NOT NULL COMMENT '毫秒精度：DATETIME(0) 会把小数秒四舍五入进未来，导致活动刚开始被判为未开始',
  `end_time`      DATETIME(3)     NOT NULL,
  `status`        TINYINT         NOT NULL DEFAULT 0 COMMENT '0未开始 1进行中 2已结束',
  `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_status_end` (`status`, `end_time`),
  KEY `idx_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ------------------------------------------------------------
-- stock_db
-- ------------------------------------------------------------
USE `stock_db`;

CREATE TABLE IF NOT EXISTS `stock` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `product_id` BIGINT UNSIGNED NOT NULL,
  `total`      INT             NOT NULL,
  `available`  INT             NOT NULL,
  `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_product` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `stock_flow` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `activity_id` BIGINT UNSIGNED NOT NULL,
  `user_id`     BIGINT UNSIGNED NOT NULL,
  `order_no`    VARCHAR(64)     NOT NULL,
  `delta`       INT             NOT NULL,
  `type`        TINYINT         NOT NULL COMMENT '1扣减 2回补',
  `created_at`  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_type` (`order_no`, `type`),
  KEY `idx_activity_user` (`activity_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ------------------------------------------------------------
-- order_db
-- ------------------------------------------------------------
USE `order_db`;

CREATE TABLE IF NOT EXISTS `order` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `order_no`    VARCHAR(64)     NOT NULL,
  `user_id`     BIGINT UNSIGNED NOT NULL,
  `activity_id` BIGINT UNSIGNED NOT NULL,
  `product_id`  BIGINT UNSIGNED NOT NULL,
  `product_name` VARCHAR(128)   NOT NULL DEFAULT '' COMMENT '下单时快照的商品名，避免商品改名后订单显示错乱',
  `amount`      BIGINT          NOT NULL,
  `status`      TINYINT         NOT NULL DEFAULT 0 COMMENT '0待支付 1已支付 2已取消',
  `expire_at`   DATETIME        NOT NULL,
  `paid_at`     DATETIME        NULL,
  `created_at`  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_no` (`order_no`),
  -- 幂等兜底：同一用户对同一活动只能有一条订单
  UNIQUE KEY `uk_user_activity` (`user_id`, `activity_id`),
  KEY `idx_status_expire` (`status`, `expire_at`),
  KEY `idx_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `mq_consume_log` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `msg_id`     VARCHAR(64)     NOT NULL,
  `consumer`   VARCHAR(64)     NOT NULL,
  `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_msg_consumer` (`msg_id`, `consumer`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ------------------------------------------------------------
-- 演示数据：1 个商品 + 1 场秒杀活动（库存 100）
-- 活动时间：建表时刻 ~ 建表时刻 + 3 天
-- ------------------------------------------------------------
USE `product_db`;

INSERT INTO `product` (`id`, `name`, `price`, `cover`, `description`, `status`)
VALUES (1, 'iPhone 30 Pro 秒杀款', 999900, 'https://example.com/iphone30.png', '演示用秒杀商品', 1);

INSERT INTO `seckill_activity`
  (`id`, `product_id`, `seckill_price`, `total_stock`, `stock`, `start_time`, `end_time`, `status`)
VALUES
  (1, 1, 99900, 100, 100, NOW() - INTERVAL 1 MINUTE, NOW() + INTERVAL 3 DAY, 1);

USE `stock_db`;

INSERT INTO `stock` (`product_id`, `total`, `available`)
VALUES (1, 100, 100);
