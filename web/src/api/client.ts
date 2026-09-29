import type { ApiEnvelope } from './types'

/**
 * 网关永远返回 HTTP 200，成败只看响应体里的 code。
 * 所以这里绝不能用 res.ok 判断业务结果，一切以 code 为准。
 */
export const CODE = {
  OK: 0,

  RATE_LIMITED: 10001,
  ALREADY_BOUGHT: 10002,
  SOLD_OUT: 10003,
  ACTIVITY_INVALID: 10004,
  UNAUTHORIZED: 10005,
  BAD_REQUEST: 10006,
  ORDER_NOT_FOUND: 10007,
  ORDER_STATE: 10008,
  CART_LIMIT: 10009,
  DUPLICATE_REQ: 10010,

  INTERNAL: 20001,
  ROLLBACK_FAILED: 20002,
} as const

/** 前端自造的传输层错误码（后端不会返回这些） */
export const CODE_NETWORK = -1
export const CODE_PROTOCOL = -2

export class ApiError extends Error {
  readonly code: number

  constructor(code: number, msg: string) {
    super(msg)
    this.name = 'ApiError'
    this.code = code
  }

  /** network：网关没起来 / 端口不通；protocol：返回体不是预期结构 */
  get isTransport(): boolean {
    return this.code === CODE_NETWORK || this.code === CODE_PROTOCOL
  }
}

/* -------------------------------------------------------------------------- */
/* 依赖注入：api 层不直接依赖 Pinia，避免 store ↔ client 循环引用              */
/* -------------------------------------------------------------------------- */

let tokenProvider: () => string | null = () => null
let onUnauthorized: (err: ApiError) => void = () => {}
let onFatal: (err: ApiError) => void = () => {}

export function setTokenProvider(fn: () => string | null): void {
  tokenProvider = fn
}

/** code=10005：登录态失效，交由 store 清理并跳登录 */
export function setUnauthorizedHandler(fn: (err: ApiError) => void): void {
  onUnauthorized = fn
}

/** code=20001/20002：后端内部错误，必须让用户看见，不能静默 */
export function setFatalHandler(fn: (err: ApiError) => void): void {
  onFatal = fn
}

/* -------------------------------------------------------------------------- */

export type QueryValue = string | number | boolean | undefined | null

/** 网关用 strconv 解析 query，空串会解析失败；这里统一丢掉空值 */
function buildUrl(path: string, query?: Record<string, QueryValue>): string {
  if (!query) return path
  const usp = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue
    usp.append(k, String(v))
  }
  const qs = usp.toString()
  return qs ? `${path}?${qs}` : path
}

/** 最近一次请求的往返时延，用于半程时延补偿（见 useServerClock） */
export const netStats = { lastRttMs: 0 }

interface RequestOptions {
  query?: Record<string, QueryValue>
  body?: unknown
}

/**
 * 返回整个响应包。
 * 绝大多数接口的 data 就是我们想要的，但 /health 这类接口直接把结果放在包上
 * （形如 {"code":0,"msg":"ok"}，没有 data），所以必须保留这一层。
 */
export async function requestEnvelope<T>(
  method: 'GET' | 'POST',
  path: string,
  opts: RequestOptions = {},
): Promise<ApiEnvelope<T>> {
  const headers: Record<string, string> = {}
  const token = tokenProvider()
  if (token) headers.Authorization = `Bearer ${token}`

  let payload: string | undefined
  if (method === 'POST') {
    headers['Content-Type'] = 'application/json'
    // 后端 bindJSON 要求 body 非空，无入参的接口也要送一个 {}
    payload = JSON.stringify(opts.body ?? {})
  }

  const started = performance.now()

  let res: Response
  try {
    res = await fetch(buildUrl(path, opts.query), { method, headers, body: payload })
  } catch (e) {
    // fetch 只有在网络层失败时才 reject（网关未启动、端口不通、CORS 拦截等）
    throw new ApiError(CODE_NETWORK, `无法连接网关：${(e as Error).message}`)
  }

  netStats.lastRttMs = performance.now() - started

  let env: ApiEnvelope<T> | null = null
  try {
    env = (await res.json()) as ApiEnvelope<T>
  } catch {
    env = null
  }

  if (!env || typeof env.code !== 'number') {
    throw new ApiError(CODE_PROTOCOL, `网关返回了非预期响应（HTTP ${res.status}）`)
  }

  if (env.code !== CODE.OK) {
    const err = new ApiError(env.code, env.msg)
    if (err.code === CODE.UNAUTHORIZED) onUnauthorized(err)
    else if (err.code === CODE.INTERNAL || err.code === CODE.ROLLBACK_FAILED) onFatal(err)
    throw err
  }

  return env
}

export async function get<T>(path: string, query?: Record<string, QueryValue>): Promise<T> {
  const env = await requestEnvelope<T>('GET', path, { query })
  return env.data as T
}

export async function post<T>(path: string, body?: unknown): Promise<T> {
  const env = await requestEnvelope<T>('POST', path, { body })
  return env.data as T
}

/** 少数接口（如 /health）不带 data，直接拿响应包 */
export function getRaw<T>(path: string, query?: Record<string, QueryValue>): Promise<ApiEnvelope<T>> {
  return requestEnvelope<T>('GET', path, { query })
}
