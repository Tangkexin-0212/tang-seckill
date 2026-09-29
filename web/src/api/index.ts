import { get, getRaw, post } from './client'
import type {
  Activity,
  ActivityCreatedData,
  ApiEnvelope,
  CartCountData,
  CartItem,
  ClearedData,
  HealthData,
  LoginData,
  Order,
  Product,
  ProductCreatedData,
  RegisterData,
  ReleasedData,
  RemovedData,
  SeckillInfo,
  SeckillResult,
  WarmedData,
} from './types'

export const userApi = {
  register: (username: string, password: string) =>
    post<RegisterData>('/api/user/register', { username, password }),
  login: (username: string, password: string) =>
    post<LoginData>('/api/user/login', { username, password }),
}

export const catalogApi = {
  products: (limit = 24, offset = 0) => get<Product[]>('/api/product/list', { limit, offset }),
  activities: (limit = 24) => get<Activity[]>('/api/activity/list', { limit }),
}

export const seckillApi = {
  info: (activityId: number) => get<SeckillInfo>('/api/seckill/info', { activityId }),
  /** 抢购。注意：返回外层的 code 才是成败，内层 SeckillResult.code 恒为 0 */
  grab: (activityId: number) => post<SeckillResult>('/api/seckill/do', { activityId }),
  cancel: (activityId: number) => post<ReleasedData>('/api/seckill/cancel', { activityId }),
}

export const cartApi = {
  list: () => get<CartItem[]>('/api/cart/list'),
  /** 条目数：服务端走 HLen，O(1)，角标场景用它而不是拉全量列表 */
  count: () => get<CartCountData>('/api/cart/count'),
  add: (productId: number, qty = 1) => post<CartItem>('/api/cart/add', { productId, qty }),
  /** qty <= 0 等同于删除该条目（后端语义） */
  update: (productId: number, qty: number) => post<CartItem>('/api/cart/update', { productId, qty }),
  remove: (productId: number) => post<RemovedData>('/api/cart/remove', { productId }),
  clear: () => post<ClearedData>('/api/cart/clear'),
}

export const orderApi = {
  list: (limit = 30) => get<Order[]>('/api/order/list', { limit }),
  get: (orderNo: string) => get<Order>('/api/order/get', { orderNo }),
  pay: (orderNo: string) => post<Order>('/api/order/pay', { orderNo }),
}

export interface CreateProductPayload {
  name: string
  price: number
  cover?: string
  description?: string
}

export interface CreateActivityPayload {
  productId: number
  totalStock: number
  seckillPrice?: number
  /** 毫秒时间戳；<=0 时后端取当前时间 */
  startTimeMs?: number
  /** 毫秒时间戳；<=0 时后端按 timeoutSeconds 推算 */
  endTimeMs?: number
  /** 秒；仅当 endTimeMs <= 0 时生效 */
  timeoutSeconds?: number
}

/** 管理侧接口目前无鉴权，仅用于实验台 */
export const adminApi = {
  createProduct: (payload: CreateProductPayload) =>
    post<ProductCreatedData>('/api/admin/product', payload),
  createActivity: (payload: CreateActivityPayload) =>
    post<ActivityCreatedData>('/api/admin/activity', payload),
  warmup: (activityId: number) => post<WarmedData>('/api/admin/warmup', { activityId }),
}

export const systemApi = {
  /** /health 直接返回响应包本身（无 data 字段），所以走 getRaw 而不是 get */
  health: (): Promise<ApiEnvelope<HealthData>> => getRaw<HealthData>('/health'),
}

export type { Activity, CartItem, Order, Product, SeckillInfo, SeckillResult }
