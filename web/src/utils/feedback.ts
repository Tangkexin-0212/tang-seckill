import { ElMessage, ElMessageBox, ElNotification } from 'element-plus'
import { describeError } from '@/api/errors'

/**
 * 统一反馈出口：把"错误码 → 界面表现"的映射（api/errors.ts）接到 Element Plus 的
 * 消息组件上。业务组件只管调 feedback.fromError(err)，不需要知道错误码细节。
 *
 * 语气分级（与 api/errors.ts 的 tone 一一对应）：
 *   ok     → ElMessage.success
 *   notice → ElMessage.info
 *   warn   → ElMessage.warning
 *   danger → ElNotification.error（后端出事必须更醒目，且不会被连续操作冲掉）
 */
/**
 * 顶栏高 56px，Element Plus 的消息默认贴顶 20px 会压住导航，
 * 所以统一下移到 66px，让它落在导航下方。
 */
const TOP_OFFSET = 66

export const feedback = {
  success(title: string, detail?: string): void {
    ElMessage.success({ message: detail ? `${title}：${detail}` : title, offset: TOP_OFFSET })
  },

  info(title: string, detail?: string): void {
    ElMessage.info({ message: detail ? `${title}：${detail}` : title, offset: TOP_OFFSET })
  },

  warning(title: string, detail?: string): void {
    ElMessage.warning({ message: detail ? `${title}：${detail}` : title, offset: TOP_OFFSET })
  },

  error(title: string, detail?: string): void {
    ElNotification.error({ title, message: detail ?? '', duration: 6000, offset: TOP_OFFSET })
  },

  /** 把任意异常映射成一条提示，并把映射结果返回给调用方做分支判断 */
  fromError(err: unknown, overrides?: { title?: string; detail?: string }) {
    const p = describeError(err)
    const title = overrides?.title ?? p.title
    const detail = overrides?.detail ?? p.detail

    if (p.tone === 'danger') this.error(title, detail)
    else if (p.tone === 'warn') this.warning(title, detail)
    else this.info(title, detail)

    return { ...p, title, detail }
  },

  /** 二次确认：用于删除/放弃这类不可逆操作 */
  async confirm(message: string, title = '操作确认', confirmText = '确定'): Promise<boolean> {
    try {
      await ElMessageBox.confirm(message, title, {
        confirmButtonText: confirmText,
        cancelButtonText: '取消',
        type: 'warning',
        draggable: true,
      })
      return true
    } catch {
      return false
    }
  },

  /** 复制到剪贴板（非安全上下文会失败，兜底给提示） */
  async copy(text: string, label = '内容'): Promise<void> {
    try {
      await navigator.clipboard.writeText(text)
      this.success(`${label}已复制`, text)
    } catch {
      this.warning('复制失败', '请手动选择后复制')
    }
  },
}
