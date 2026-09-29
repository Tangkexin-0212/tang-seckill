import { ref } from 'vue'

/**
 * 半程时延补偿（不是跨机时钟同步）。
 *
 * 事实：网关响应体里没有任何服务器时间戳字段，所以前端拿不到"服务器现在几点"，
 * 无法做真正的 NTP 式时钟对齐。但有一个系统性滞后是可以消掉的：
 * 当我们收到响应的那一刻，服务器其实已经又往前走了大约 rtt/2。
 * 抢购这种对秒级边界敏感的场景里，这半程时延足以让"还差 1 秒"变成"已经开抢"。
 *
 * 所以这里只补偿 rtt/2，并设 500ms 上限，避免一次网络抖动把倒计时带偏。
 */
const latencyOffsetMs = ref(0)

export function recordLatency(rttMs: number): void {
  latencyOffsetMs.value = Math.max(0, Math.min(rttMs / 2, 500))
}

export function latencyOffset(): number {
  return latencyOffsetMs.value
}

/** 估算的"服务器此刻"（本地时钟 + 半程时延补偿） */
export function serverNow(): number {
  return Date.now() + latencyOffsetMs.value
}

/** 全局共享的秒级时钟：所有卡片/订单倒计时共用一个 ref，避免 N 个定时器互相打架 */
const nowRef = ref(serverNow())
let timer: number | undefined

function ensureTicking(): void {
  if (timer !== undefined) return
  timer = window.setInterval(() => {
    nowRef.value = serverNow()
  }, 500)
}

export function useServerNow() {
  ensureTicking()
  return nowRef
}
