import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { RouteLocationNormalizedLoaded } from 'vue-router'

export interface TagItem {
  /** 用于去重的唯一键：带参数的页面按完整路径区分 */
  key: string
  path: string
  title: string
  /** 固定标签不可关闭（首页） */
  affix: boolean
}

/**
 * 标签页（TagsView）。
 * gin-vue-admin 的核心交互之一：打开过的页面留成可关闭的标签，
 * 在多页之间反复横跳时不必回到菜单重新点。
 */
export const useTags = defineStore('tags', () => {
  const visited = ref<TagItem[]>([])

  function ensure(route: RouteLocationNormalizedLoaded): void {
    const title = route.meta.title as string | undefined
    if (!title || route.meta.hidden === true) return

    const key = route.path
    if (visited.value.some((t) => t.key === key)) return

    visited.value.push({
      key,
      path: route.path,
      title,
      affix: route.meta.affix === true,
    })
  }

  function close(key: string): TagItem | null {
    const target = visited.value.find((t) => t.key === key)
    if (!target || target.affix) return null
    visited.value = visited.value.filter((t) => t.key !== key)
    return target
  }

  function closeOthers(key: string): void {
    visited.value = visited.value.filter((t) => t.affix || t.key === key)
  }

  function closeAll(): void {
    visited.value = visited.value.filter((t) => t.affix)
  }

  function reset(): void {
    visited.value = []
  }

  return { visited, ensure, close, closeOthers, closeAll, reset }
})
