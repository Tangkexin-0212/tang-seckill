namespace go order

struct Order {
    1: string orderNo
    2: i64 userId
    3: i64 activityId
    4: i64 productId
    5: string productName
    6: i64 amount
    7: i32 status
    8: i64 expireAtMs
    9: i64 createdAtMs
    10: i64 paidAtMs
}

struct CreatePendingReq {
    1: i64 userId
    2: i64 activityId
    3: i64 productId
    4: string productName
    5: i64 amount
    6: i64 ttlSeconds
}

service OrderService {
    // 幂等创建待支付订单：同一 (userId, activityId) 重复调用返回同一订单
    Order CreatePending(1: CreatePendingReq req)
    // 模拟支付：pending -> paid，重复支付返回已支付状态
    Order Pay(1: string orderNo)
    // 取消订单：pending -> cancelled
    bool Cancel(1: string orderNo, 2: string reason)
    Order GetOrder(1: string orderNo)
    // 按 (用户, 活动) 查订单：主动取消场景需要拿到 orderNo
    Order GetOrderByUserActivity(1: i64 userId, 2: i64 activityId)
    list<Order> ListOrders(1: i64 userId, 2: i32 limit)
}
