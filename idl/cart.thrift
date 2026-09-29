namespace go cart

// 购物车条目（存储形态：Redis Hash  sk:cart:{uid}  field=productId  value=qty）
struct CartItem {
    1: i64 userId
    2: i64 productId
    3: i32 qty
    4: string productName
    5: string cover
    6: i64 originalPrice
    7: i64 price
    8: i64 activityId
}

service CartService {
    // 普通加购：存在则累加（有单件上限校验）
    CartItem AddItem(1: i64 userId, 2: i64 productId, 3: i32 qty)

    // 秒杀加购：幂等，无论调用多少次 qty 恒为 1
    CartItem SeckillAdd(1: i64 userId, 2: i64 productId, 3: i64 activityId,
                        4: i64 price, 5: string productName, 6: string cover)

    // 修改数量，qty<=0 视为删除
    CartItem UpdateQty(1: i64 userId, 2: i64 productId, 3: i32 qty)

    bool RemoveItem(1: i64 userId, 2: i64 productId)
    bool ClearCart(1: i64 userId)
    list<CartItem> ListCart(1: i64 userId)
    i32 CountItems(1: i64 userId)
}
