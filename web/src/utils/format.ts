/**
 * 展示层格式化。所有金额入参单位是「分」，所有时间入参单位是「毫秒」。
 */

/** 1999_00 分 → "¥1,999.00" */
export function money(cents: number | undefined | null): string {
  const v = Number(cents ?? 0)
  const sign = v < 0 ? '-' : ''
  const abs = Math.abs(v)
  const yuan = Math.floor(abs / 100)
  const fen = abs % 100
  return `${sign}¥${yuan.toLocaleString('zh-CN')}.${String(fen).padStart(2, '0')}`
}

/** 只要数字部分，用于把 "¥" 拆成独立的小号字排版 */
export function moneyParts(cents: number | undefined | null): { unit: string; int: string; frac: string } {
  const v = Math.abs(Number(cents ?? 0))
  return {
    unit: '¥',
    int: Math.floor(v / 100).toLocaleString('zh-CN'),
    frac: String(v % 100).padStart(2, '0'),
  }
}

/** 折扣率文案，例如 99900 / 999900 → "1.0 折" */
export function discountLabel(seckillPrice: number, originalPrice: number): string {
  if (!originalPrice || originalPrice <= 0 || seckillPrice <= 0) return ''
  const ratio = (seckillPrice / originalPrice) * 10
  return `${ratio.toFixed(ratio < 1 ? 2 : 1)} 折`
}

/** 剩余毫秒 → "02:13:45"（超过 24h 时前缀天数） */
export function duration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const d = Math.floor(total / 86400)
  const h = Math.floor((total % 86400) / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const hms = `${pad(h)}:${pad(m)}:${pad(s)}`
  return d > 0 ? `${d}天 ${hms}` : hms
}

/** 剩余毫秒 → { mm, ss, cs }，给巨型倒计时拨盘用 */
export function splitCountdown(ms: number): {
  days: string
  hours: string
  minutes: string
  seconds: string
  centis: string
} {
  const total = Math.max(0, ms)
  const d = Math.floor(total / 86400000)
  const h = Math.floor((total % 86400000) / 3600000)
  const m = Math.floor((total % 3600000) / 60000)
  const s = Math.floor((total % 60000) / 1000)
  const cs = Math.floor((total % 1000) / 10)
  return {
    days: String(d),
    hours: pad(h),
    minutes: pad(m),
    seconds: pad(s),
    centis: pad(cs),
  }
}

/** 毫秒时间戳 → "2026-09-28 20:31:07" */
export function dateTime(ms: number | undefined | null): string {
  if (!ms) return '—'
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(
    d.getMinutes(),
  )}:${pad(d.getSeconds())}`
}

/** 毫秒时间戳 → "09-28 20:31"，表格里用的紧凑格式（不含年份） */
export function dateTimeShort(ms: number | undefined | null): string {
  if (!ms) return '—'
  const d = new Date(ms)
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 毫秒时间戳 → "20:31:07" */
export function clockTime(ms: number | undefined | null): string {
  if (!ms) return '—'
  const d = new Date(ms)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 相对时间："3 分钟前" */
export function relative(ms: number | undefined | null): string {
  if (!ms) return '—'
  const diff = Date.now() - ms
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

/** 订单号尾段，用于紧凑展示 */
export function shortOrderNo(orderNo: string): string {
  if (!orderNo) return '—'
  return orderNo.length <= 14 ? orderNo : `…${orderNo.slice(-13)}`
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}
