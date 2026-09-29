# 秒杀系统（Kitex 微服务）

基于 **CloudWeGo Kitex**（Thrift RPC，etcd 服务发现）+ **Hertz**（HTTP 网关）+ **Redis Lua**（限流 / 扣减 / 购物车）
实现的秒杀系统，重点解决四件事：**限流**、**活动预热同步**、**Redis 购物车**、**幂等与一人一件**。

---

## 1. 需求 → 实现对照

| 需求 | 实现位置 | 说明 |
| --- | --- | --- |
| 秒杀接口 + 限流（Redis 令牌桶） | `seckill/`、`pkg/lua/token_bucket.lua`、`seckill/limiter.go` | 用户 / 商品 / 全局三层令牌桶，Redis 故障时 Fail-Open |
| 定时任务同步 MySQL → Redis（cobra） | `cron/` | cobra 子命令 + robfig/cron 调度，`SETNX` 初始化库存绝不覆盖 |
| 购物车接口（Redis Hash） | `cart/`、`pkg/lua/cart_*.lua` | `sk:cart:{uid}` 存数量 + `sk:cart:snap:{uid}` 存商品快照 |
| 接口幂等 / 一人一件 | `pkg/lua/seckill.lua`、`order_db.order` 唯一索引、`pkg/lua/cart_seckill_add.lua` | Redis Set 挡重复 + MySQL 唯一索引兜底 + 秒杀加购 `HSETNX` 保证 qty 恒为 1 |

---

## 2. 架构

```
                     ┌──────────────────────────────┐
   浏览器 / 压测脚本  │  gateway (Hertz, :8080)      │  统一鉴权 + 参数校验
                     └──────────────┬───────────────┘
                                    │ Kitex RPC (etcd 服务发现)
        ┌───────────────┬───────────┼───────────┬───────────────┐
        ▼               ▼           ▼           ▼               ▼
   user (:9001)   product(:9002)  cart(:9004)  order(:9006)  stock(:9003)
   user_db        product_db      Redis Hash   order_db      stock_db
                                        ▲
                                        │  原子扣减 / 限流 / 加购
                     ┌──────────────────┴──────────────────┐
                     │  seckill (:9005)  秒杀编排           │
                     │  限流 → 扣减 → 加购 → 建单 → 投事件  │
                     └──────────────────┬──────────────────┘
                                        ▼
                              Redis Stream / ZSet
                                        │
                     ┌──────────────────┴──────────────────┐
                     │  worker  落库消费 + 超时释放扫描      │
                     │  cron    活动同步 + 库存对账 (cobra)  │
                     └─────────────────────────────────────┘
```

### 一次抢购的完整链路

1. **鉴权**：网关校验 token（`user.VerifyToken`），把 `uid` 注入上下文，下游只信任网关传入的 `userId`
2. **读快照**：`seckill` 从 Redis 读活动快照 `sk:seckill:act:{id}`，校验活动时间窗
3. **限流**：三层令牌桶（用户 5 容量 / 1 QPS、商品维度按库存自适应、全局兜底 5000）
4. **原子扣减**：`sk:seckill.lua` 一次性完成 `SISMEMBER 判重 → DECR 库存 → SADD 记录资格`
5. **幂等加购**：`cart.SeckillAdd` 用 `HSETNX` 写入，重复调用数量恒为 1，不会累加
6. **创建待支付订单**：`order.CreatePending`，`uk(user_id, activity_id)` 是 DB 侧"一人一件"的最终兜底，
   同时把 `orderNo` 写入 ZSet 延时队列
7. **投递落库事件**：写入 Redis Stream，由 `worker` 异步补 `stock_flow` 流水与 `stock.available`
   —— **Redis 是权威库存，MySQL 最终一致**

任一步失败都会调用 `RollbackWithoutOrder`：归还库存 + 删除幂等标记，用户可以重新抢，库存不会被凭空吞掉。

---

## 3. 目录结构

```
seckill/
├── idl/                    6 份 Thrift IDL
├── kitex_gen/              kitex 生成的代码
├── pkg/
│   ├── bootstrap/          统一进程启动（配置 + 日志 + MySQL + Redis）
│   ├── config/             YAML 配置
│   ├── consts/             服务名 / 库名 / Redis Key 命名
│   ├── errno/              业务错误码 + Kitex BizStatusError 互转
│   ├── lua/                6 段 Lua 脚本（go:embed）
│   ├── models/             GORM 模型
│   ├── mysqlx/ redisx/     MySQL / Redis 封装（含脚本缓存）
│   ├── registryx/          etcd 注册与发现
│   ├── rpcx/               Kitex Server/Client 选项与客户端聚合
│   ├── seckillkit/         活动快照同步 / 库存释放 / 事件定义（三处共用）
│   └── utils/
├── user/ product/ stock/ cart/ order/ seckill/   六个领域服务
├── gateway/                Hertz HTTP 网关
├── cron/                   cobra 定时任务（同步 / 对账 / 清理）
├── worker/                 Stream 消费者 + 延时队列扫描
├── tools/loadtest/         1000 并发压测与验收脚本
├── scripts/                start-all.ps1 / stop-all.ps1
├── sql/01_schema.sql       建库建表 + 演示数据
└── docker-compose.yml      MySQL + Redis + etcd
```

---

## 4. 快速开始

### 4.1 启动

```powershell
# 一键：拉起容器 + 编译 + 启动全部进程
powershell -ExecutionPolicy Bypass -File .\scripts\start-all.ps1

# 只重启应用进程（容器已在跑）
powershell -ExecutionPolicy Bypass -File .\scripts\start-all.ps1 -SkipInfra

# 停止
powershell -ExecutionPolicy Bypass -File .\scripts\stop-all.ps1
```

日志在 `logs/` 目录，每个进程一份 `.log` 与 `.err.log`。

### 4.2 手工启动单个服务

```powershell
go run ./user -config config.yaml        # 其余服务同理
go run ./cron                            # 启动调度器
go run ./cron sync                       # 手动执行一次同步
go run ./cron reconcile --fix            # 手动对账并修复
```

### 4.3 压测与验收

```powershell
# 1000 并发用户抢 100 库存，自动断言全部验收标准
go run ./tools/loadtest -users 1000 -stock 100
```

脚本会自动：创建商品与活动 → 预热 → 注册登录 1000 个用户 → **同一时刻并发发起请求** →
统计成功/失败分布与延迟分位 → 校验购物车 qty → 重复请求验证幂等 → 轮询 MySQL 验证最终一致，
最后输出 `验收通过` 或逐条失败原因（退出码非 0）。

---

## 5. HTTP 接口

响应统一为 `{"code":0,"msg":"success","data":{...}}`，**HTTP 状态码恒为 200**，业务结果看 `code`。

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/health` | - | 健康检查 |
| POST | `/api/user/register` | - | 注册 `{username,password}` |
| POST | `/api/user/login` | - | 登录，返回 `{token,uid,expireAtMs}` |
| GET | `/api/product/list` | - | 商品列表 |
| GET | `/api/activity/list` | - | 活动列表 |
| GET | `/api/seckill/info?activityId=` | - | 活动详情（纯 Redis 读） |
| POST | `/api/seckill/do` | ✔ | **秒杀** `{activityId}` |
| POST | `/api/seckill/cancel` | ✔ | 主动取消并归还库存 |
| GET | `/api/cart/list` | ✔ | 购物车列表 |
| POST | `/api/cart/add` | ✔ | 加购 `{productId,qty}` |
| POST | `/api/cart/update` | ✔ | 改数量 `{productId,qty}`（≤0 删除） |
| POST | `/api/cart/remove` | ✔ | 删除 `{productId}` |
| POST | `/api/cart/clear` | ✔ | 清空购物车 |
| POST | `/api/order/pay` | ✔ | 支付 `{orderNo}` |
| GET | `/api/order/list` | ✔ | 我的订单 |
| GET | `/api/order/get?orderNo=` | ✔ | 订单详情 |
| POST | `/api/admin/product` | - | 建商品（顺手初始化库存） |
| POST | `/api/admin/activity` | - | 建活动（顺手初始化活动库存） |
| POST | `/api/admin/warmup` | - | 手动预热 `{activityId}` |

鉴权头：`Authorization: Bearer <token>`。

### 错误码

| code | 含义 |
| --- | --- |
| 0 | 成功 |
| 10001 | 被限流，请稍后再试 |
| 10002 | 已参与过该活动（幂等命中，消息里带原订单号） |
| 10003 | 库存不足 / 已售罄 |
| 10004 | 活动无效（未开始、已结束或未预热） |
| 10005 | 未登录或 token 失效 |
| 10009 | 购物车数量超限 |
| 10010 | 服务内部异常 |

---

## 6. 核心设计说明

### 6.1 令牌桶限流（`pkg/lua/token_bucket.lua`）

Hash 存 `tokens` 与 `ts`，每次请求按 `(now - ts) * rate` 补桶后取令牌。
相比固定窗口计数，它**没有临界点双倍流量**问题（窗口边界前后各放一批，瞬时可达 2 倍阈值）。

三层维度：

| 维度 | Key | 参数 |
| --- | --- | --- |
| 用户 | `sk:rl:user:{uid}` | 容量 5，1 QPS —— 挡住单账号脚本刷单 |
| 商品 | `sk:rl:act:{actId}` | `容量 = 总库存 × 1.5`，`速率 = 总库存 / 活动时长 × 3` —— 桶足够大让真实流量进来，速率限制瞬时洪峰 |
| 全局 | `sk:rl:global` | 容量 5000，3000 QPS —— 兜底保护 Redis 与下游 |

Redis 异常时按 `failOpen: true` **放行**：限流是保护措施，不应成为可用性单点。

### 6.2 库存与幂等（`pkg/lua/seckill.lua`）

```lua
-- 三步原子完成，杜绝"判重过了但库存没扣"这类中间态
if SISMEMBER(usersKey, uid) == 1 then return {-1, stock} end   -- 已抢过
if not EXISTS(stockKey) then return {-3, 0} end                -- 未预热
if tonumber(stock) <= 0 then return {-2, 0} end                -- 售罄
DECR(stockKey); SADD(usersKey, uid)                            -- 扣减 + 记录资格
return {1, stock - 1}
```

三层幂等保障：

1. **Redis Set**：`sk:seckill:user:{actId}` 在 Lua 内原子判定，挡住 99.9% 的重复请求
2. **MySQL 唯一索引**：`order.uk(user_id, activity_id)`，即使 Redis 被清空也只会有一条订单
3. **购物车 HSETNX**：秒杀加购强制 qty=1，重复执行不会变成 2、3、4

### 6.3 最终一致性

```
Redis（权威）: 扣库存 → SADD 资格 → XADD 事件
                                      │
worker: XReadGroup → mq_consume_log 幂等 → stock.Deduct（uk(order_no,type) 二次幂等）→ XACK
                                      │
失败 → 重试计数（sk:mq:retry:{id}）→ 超过 maxRetry 进死信流 sk:stream:seckill:dlq
```

同步链路完全不碰 MySQL，因此**抢购吞吐只受 Redis 限制**。

### 6.4 超时释放（延时队列）

- 建单时 `ZADD sk:order:delay score=expireAtMs member=orderNo`
- 支付时 `ZREM`（摘除后延时任务不再释放）
- `worker` 每 200ms 扫描到期订单，调用 `release.lua`

Lua 内**用 `ZREM` 的返回值做并发互斥**：只有真正摘除成功的那个执行者才回补库存，
因此多实例扫描是安全的，也不会出现"支付成功同时被释放"的双写。

### 6.5 活动同步（cron）

- `sync`：每 30s 把 MySQL 中未结束的活动写进 Redis。库存用 `SETNX`——**已预热的活动绝不覆盖**，
  否则会把用户已扣减的库存刷回去直接造成超卖
- `reconcile`：每 1min 核对 `Redis 剩余库存 + MySQL 已扣减 == 总库存`。
  由于落库异步，短时间"Redis 扣得更多"是在途状态，只告警；`--fix` 才修复真实异常
- `cleanup`：清理已结束活动的 Redis 残留

---

## 7. 验收结果

`go run ./tools/loadtest -users 1000 -stock 100` 断言项：

| 断言 | 期望 |
| --- | --- |
| 成功数 | 恰好 100 |
| 失败数 | 900（10002 / 10003 / 10001） |
| 超卖 | Redis 剩余库存 = 0 |
| 一人一件 | 成功用户购物车中该商品 qty 恒为 1 |
| 幂等 | 重复请求返回 10002 且库存不变 |
| 最终一致 | MySQL `stock_flow` 扣减流水 = 100，`stock.available` = 0 |

---

## 8. 配置要点（`config.yaml`）

| 配置 | 默认 | 说明 |
| --- | --- | --- |
| `order.payTimeoutSeconds` | 900 | 待支付超时。**验证释放逻辑时改成 3**，秒级就能看到库存回补 |
| `order.delayScanIntervalMs` | 200 | 延时队列扫描间隔 |
| `ratelimit.failOpen` | true | Redis 异常时是否放行 |
| `cron.syncSpec` | `*/30 * * * * *` | 同步周期（6 段含秒） |
| `worker.maxRetry` | 3 | 单条事件最大重试次数，超过进死信 |

---

## 9. 设计取舍

- **购物车全量 Redis Hash**：无 MySQL 表，`cart_db` 仅作为占位库保留。购物车是高频低价值数据，
  用 `Hash(qty) + Hash(快照)` 两个 key 支撑列表展示，避免读 DB
- **额外引入 order 与 seckill 服务**：需求中的四个服务（用户/商品/库存/购物车）是领域划分，
  但"超时释放库存"必须有订单状态承载，而"限流 → 扣减 → 加购 → 建单"的编排也不适合塞进任一领域服务，
  因此拆出 `order`（订单状态机 + 延时队列）与 `seckill`（编排 + 限流 + 幂等聚合）
- **单 module 多进程**：本地开发与压测更方便，`go build ./...` 一次编译全部；生产可平滑拆成多仓库
- **`kitex_gen` 由 Thrift 生成**：修改 IDL 后重新执行 `kitex -module seckill idl/<name>.thrift` 即可
