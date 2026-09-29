namespace go stock

struct StockInfo {
    1: i64 productId
    2: i32 total
    3: i32 available
}

service StockService {
    StockInfo GetStock(1: i64 productId)
    bool InitStock(1: i64 productId, 2: i32 total)

    // DB 侧扣减（兜底链路），orderNo 保证幂等
    bool Deduct(1: i64 activityId, 2: i64 userId, 3: i64 productId, 4: string orderNo)
    // DB 侧回补
    bool Rollback(1: i64 activityId, 2: i64 userId, 3: i64 productId, 4: string orderNo)

    // 某活动在 DB 中的已扣减数量，用于对账
    i32 DeductedCount(1: i64 activityId)
}
