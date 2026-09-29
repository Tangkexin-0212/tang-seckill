namespace go product

struct Product {
    1: i64 id
    2: string name
    3: i64 price
    4: string cover
    5: string description
    6: i32 status
}

struct SeckillActivity {
    1: i64 id
    2: i64 productId
    3: i64 seckillPrice
    4: i32 totalStock
    5: i32 stock
    6: i64 startTimeMs
    7: i64 endTimeMs
    8: i32 status
    9: string productName
    10: string cover
    11: i64 originalPrice
}

// cron 同步任务的输入行
struct SeckillSyncItem {
    1: i64 activityId
    2: i64 productId
    3: string productName
    4: string cover
    5: i64 originalPrice
    6: i64 seckillPrice
    7: i32 totalStock
    8: i32 stock
    9: i64 startTimeMs
    10: i64 endTimeMs
    11: i32 status
}

service ProductService {
    i64 CreateProduct(1: string name, 2: i64 price, 3: string cover, 4: string description)
    Product GetProduct(1: i64 productId)
    list<Product> ListProducts(1: i32 limit, 2: i32 offset)

    i64 CreateActivity(1: i64 productId, 2: i64 seckillPrice, 3: i32 totalStock, 4: i64 startTimeMs, 5: i64 endTimeMs)
    SeckillActivity GetActivity(1: i64 activityId)
    list<SeckillActivity> ListActivities(1: i32 limit)

    // 供 cron 拉取需要同步到 Redis 的活动（未开始 + 进行中 且未过期）
    list<SeckillSyncItem> ListSyncTargets(1: i64 nowMs)
    // 活动结束
    bool FinishActivity(1: i64 activityId)
    // 同步后回写活动状态
    bool UpdateActivityStatus(1: i64 activityId, 2: i32 status)
}
