import { ApiError, CODE, CODE_NETWORK, CODE_PROTOCOL } from './client'

/**
 * 错误码 → 界面表现的唯一映射处。
 *
 * 设计取舍：10001~10010 这些"用户自己在秒杀里撞上的墙"不该用红色报错弹窗，
 * 它们是这个产品的正常剧情（手慢了、抢过了、售罄了），所以给 notice/warn 语气；
 * 只有 20001/20002 以及传输层失败才是真的"系统坏了"，用 danger 并要求用户看见。
 */
export type ErrorTone = 'notice' | 'warn' | 'danger'

export interface ErrorPresentation {
  tone: ErrorTone
  title: string
  detail: string
  code: number
}

export function toApiError(err: unknown): ApiError {
  if (err instanceof ApiError) return err
  const message = err instanceof Error ? err.message : String(err)
  return new ApiError(CODE_PROTOCOL, message)
}

export function describeError(err: unknown): ErrorPresentation {
  const e = toApiError(err)
  const code = e.code

  switch (code) {
    case CODE_NETWORK:
      return {
        tone: 'danger',
        code,
        title: '连不上网关',
        detail: '确认后端已启动（npm 侧走 Vite 代理 → 127.0.0.1:8080）。',
      }
    case CODE_PROTOCOL:
      return {
        tone: 'danger',
        code,
        title: '响应无法解析',
        detail: e.message || '网关返回了非预期结构，检查代理是否指到了 gateway。',
      }
    case CODE.RATE_LIMITED:
      return {
        tone: 'warn',
        code,
        title: '被限流了',
        detail: '令牌桶见底，冷却几秒后可以再试——这是限流在正常工作。',
      }
    case CODE.ALREADY_BOUGHT:
      return {
        tone: 'notice',
        code,
        title: '你已经抢到过这一件',
        detail: e.message || '每人限购 1 件，重复请求会命中幂等判定。',
      }
    case CODE.SOLD_OUT:
      return { tone: 'notice', code, title: '已售罄', detail: '库存被抢空了，换一场试试。' }
    case CODE.ACTIVITY_INVALID:
      return {
        tone: 'warn',
        code,
        title: '活动不可参与',
        detail: e.message || '活动不存在、未开始或已结束。',
      }
    case CODE.UNAUTHORIZED:
      return { tone: 'warn', code, title: '登录态已失效', detail: 'Token 过期或无效，请重新登录。' }
    case CODE.BAD_REQUEST:
      return { tone: 'warn', code, title: '参数不合法', detail: e.message || '请检查填写内容。' }
    case CODE.ORDER_NOT_FOUND:
      return { tone: 'warn', code, title: '订单不存在', detail: '可能被释放或不是当前账号的订单。' }
    case CODE.ORDER_STATE:
      return {
        tone: 'warn',
        code,
        title: '订单状态不允许该操作',
        detail: e.message || '订单可能已支付或已取消。',
      }
    case CODE.CART_LIMIT:
      return { tone: 'warn', code, title: '购物车超出上限', detail: e.message || '请减少数量后再试。' }
    case CODE.DUPLICATE_REQ:
      return { tone: 'notice', code, title: '重复请求被拦下', detail: '同一请求短时间内重复提交。' }
    case CODE.INTERNAL:
      return {
        tone: 'danger',
        code,
        title: '后端内部错误',
        detail: e.message || '系统繁忙，这不是你的问题，去看 seckill/worker 日志。',
      }
    case CODE.ROLLBACK_FAILED:
      return {
        tone: 'danger',
        code,
        title: '库存回补失败',
        detail: e.message || '需要人工核对 Redis 与 MySQL 的库存差异。',
      }
    default:
      return {
        tone: 'danger',
        code,
        title: `未知错误 ${code}`,
        detail: e.message || '请查看后端日志。',
      }
  }
}

/** 是否属于"用户操作成功但业务上说不行"的幂等类结果，UI 可借此切换按钮状态 */
export function isExpectedOutcome(code: number): boolean {
  return (
    code === CODE.RATE_LIMITED ||
    code === CODE.ALREADY_BOUGHT ||
    code === CODE.SOLD_OUT ||
    code === CODE.ACTIVITY_INVALID
  )
}

/**
 * 从"已参与"错误的 msg 里抠出订单号。
 * 后端在 10002 时会带上 orderNo，前端据此直接给出支付入口。
 */
export function extractOrderNo(text: string | undefined): string {
  if (!text) return ''
  const m = text.match(/SK[0-9a-zA-Z]+/)
  return m ? m[0] : ''
}
