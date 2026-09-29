/**
 * 后端状态枚举 → el-tag 类型 / 颜色。
 * 取值来自 sql/01_schema.sql 的列注释，不要凭感觉改：
 *   product.status      1 上架 / 0 下架
 *   seckill_activity    0 未开始 / 1 进行中 / 2 已结束
 *   order.status        0 待支付 / 1 已支付 / 2 已取消
 *
 * 语义色约定（与设计令牌一致）：
 *   在槌 / 已支付 / 库存充足 → 绿
 *   候场 / 已取消 / 已结束   → 灰
 *   待支付 / 库存告急        → 橙
 *   售罄 / 不可参与          → 红
 */
export type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger'

export interface StatusMeta {
  label: string
  type: TagType
}

export function activityStatusMeta(status: number): StatusMeta {
  switch (status) {
    case 1:
      return { label: '在槌', type: 'success' }
    case 2:
      return { label: '已落槌', type: 'info' }
    default:
      return { label: '候场', type: 'info' }
  }
}

export function orderStatusMeta(status: number): StatusMeta {
  switch (status) {
    case 1:
      return { label: '已支付', type: 'success' }
    case 2:
      return { label: '已取消', type: 'info' }
    default:
      return { label: '待支付', type: 'warning' }
  }
}

/**
 * 活动阶段以 startTimeMs/endTimeMs 重新推导，status 只作兜底：
 * 后端 status 由定时任务同步，活动刚开始/刚结束的那几秒可能滞后。
 */
export type ActivityPhase = 'upcoming' | 'live' | 'ended'

export function activityPhase(
  startTimeMs: number,
  endTimeMs: number,
  status: number,
  nowMs: number,
): ActivityPhase {
  if (endTimeMs > 0 && nowMs >= endTimeMs) return 'ended'
  if (startTimeMs > 0 && nowMs < startTimeMs) return 'upcoming'
  if (status === 2) return 'ended'
  if (status === 1) return 'live'
  return nowMs >= startTimeMs ? 'live' : 'upcoming'
}

export function phaseMeta(phase: ActivityPhase): StatusMeta {
  switch (phase) {
    case 'live':
      return { label: '在槌', type: 'success' }
    case 'ended':
      return { label: '已落槌', type: 'info' }
    default:
      return { label: '候场', type: 'info' }
  }
}

/** 库存进度条颜色：充足绿 / 告急橙 / 售罄红，让表格扫一眼就知道哪场要抢完了 */
export function stockColor(remain: number, total: number): string {
  if (total <= 0 || remain <= 0) return '#e34d59'
  const ratio = remain / total
  if (ratio <= 0.2) return '#e6a23c'
  return '#18a058'
}

/** 库存百分比（用于进度条），total 为 0 时按 0 处理 */
export function stockPercent(remain: number, total: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((remain / total) * 100)))
}
