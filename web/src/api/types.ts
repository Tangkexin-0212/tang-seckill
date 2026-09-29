/**
 * 与 gateway / Kitex IDL 严格对应的数据结构。
 * 金额单位统一为「分」，时间戳统一为「毫秒」。
 */

/** 网关统一响应包。注意 code 带 omitempty 的只有 data。 */
export interface ApiEnvelope<T> {
  code: number
  msg: string
  data?: T
}

export interface RegisterData {
  uid: number
}

export interface LoginData {
  token: string
  uid: number
  expireAtMs: number
}

/** product.status：1 上架 / 0 下架 */
export interface Product {
  id: number
  name: string
  price: number
  cover: string
  description: string
  status: number
}

/** activity.status：0 未开始 / 1 进行中 / 2 已结束 */
export interface Activity {
  id: number
  productId: number
  seckillPrice: number
  totalStock: number
  stock: number
  startTimeMs: number
  endTimeMs: number
  status: number
  productName: string
  cover: string
  originalPrice: number
}

export interface SeckillInfo {
  activityId: number
  productId: number
  productName: string
  cover: string
  originalPrice: number
  seckillPrice: number
  totalStock: number
  remainStock: number
  startTimeMs: number
  endTimeMs: number
  status: number
  soldOut: boolean
}

/**
 * 秒杀结果。data 内部自带一层 code/msg，但它恒为 0/"抢购成功"，
 * 业务成败一律以外层响应包的 code 为准。
 */
export interface SeckillResult {
  code: number
  msg: string
  orderNo: string
  remainStock: number
  activityId: number
  userId: number
}

export interface CartItem {
  userId: number
  productId: number
  qty: number
  productName: string
  cover: string
  originalPrice: number
  price: number
  activityId: number
}

/** order.status：0 待支付 / 1 已支付 / 2 已取消（库存已释放） */
export interface Order {
  orderNo: string
  userId: number
  activityId: number
  productId: number
  productName: string
  amount: number
  status: number
  expireAtMs: number
  createdAtMs: number
  paidAtMs: number
}

export interface RemovedData {
  removed: boolean
}

/** GET /api/cart/count：条目数（Redis HLen 口径），不是 qty 之和 */
export interface CartCountData {
  count: number
}

export interface ClearedData {
  cleared: boolean
}

export interface ReleasedData {
  released: boolean
}

export interface WarmedData {
  warmed: boolean
}

export interface ProductCreatedData {
  productId: number
}

export interface ActivityCreatedData {
  activityId: number
}

export interface HealthData {
  code: number
  msg: string
}
