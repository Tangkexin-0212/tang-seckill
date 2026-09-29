import { defineStore } from 'pinia'
import { ref } from 'vue'

const SIDEBAR_KEY = 'sk.sidebar.collapsed'

/** 全局界面状态：侧边栏折叠（持久化）+ 内容区刷新令牌 */
export const useApp = defineStore('app', () => {
  const collapsed = ref(localStorage.getItem(SIDEBAR_KEY) === '1')
  /** 变化时强制重建 RouterView 里的组件，等价于"刷新当前页" */
  const refreshKey = ref(0)

  function toggle(): void {
    collapsed.value = !collapsed.value
    localStorage.setItem(SIDEBAR_KEY, collapsed.value ? '1' : '0')
  }

  function refresh(): void {
    refreshKey.value += 1
  }

  return { collapsed, refreshKey, toggle, refresh }
})
