namespace go seckill

// 注意：不要命名为 SeckillResult，kitex 会与方法返回包装类型冲突而被重命名为 SeckillResult_
struct SeckillResp {
    1: i32 code
    2: string msg
    3: string orderNo
    4: i32 remainStock
    5: i64 activityId
    6: i64 userId
}

struct SeckillInfo {
    1: i64 activityId
    2: i64 productId
    3: string productName
    4: string cover
    5: i64 originalPrice
    6: i64 seckillPrice
    7: i32 totalStock
    8: i32 remainStock
    9: i64 startTimeMs
    10: i64 endTimeMs
    11: i32 status
    12: bool soldOut
}

service SeckillService {
    // 秒杀主接口：限流 -> 幂等校验 -> 原子扣库存 -> 加购物车 -> 创建待支付订单 -> 投递延时队列
    SeckillResp Seckill(1: i64 userId, 2: i64 activityId)

    // 活动页信息（直接读 Redis 快照，不回源 MySQL）
    SeckillInfo GetSeckillInfo(1: i64 activityId)

    // 手动取消/释放（回滚演示）
    bool CancelSeckill(1: i64 userId, 2: i64 activityId)

    // 手动预热单个活动（复用 cron 的同步逻辑）
    bool WarmUp(1: i64 activityId)
}
