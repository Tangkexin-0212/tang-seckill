import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { cartApi } from '@/api'
import type { CartItem } from '@/api/types'

/**
 * 购物车真身在 Redis（sk:cart:{uid}），这里只是本地镜像：
 * - 顶栏角标要立刻响应，不能每次点击都等一次网络往返；
 * - 每次写操作都用服务端返回的条目覆盖本地，保证数量/价格与后端一致。
 */
export const useCart = defineStore('cart', () => {
  const items = ref<CartItem[]>([])
  const loading = ref(false)
  /** 正在变更的 productId：让按钮能单独转圈，而不是整页 loading */
  const busyId = ref<number | null>(null)

  /**
   * 条目数（= Redis HLen，来自 /api/cart/count）：O(1)，用于侧栏/顶栏角标。
   * 注意与 count（件数）的区别：买了 3 件同一个商品时，条目数是 1、件数是 3。
   * 界面上两者必须同时出现口径说明，否则角标和购物车页会互相打脸。
   */
  const badgeCount = ref(0)

  /** 件数（qty 之和）：只有加载过明细才有意义，用于购物车页的汇总 */
  const count = computed(() => items.value.reduce((sum, i) => sum + i.qty, 0))
  const subtotal = computed(() => items.value.reduce((sum, i) => sum + i.price * i.qty, 0))
  /** 相对原价省下的钱，秒杀购物车里这个数字才是"爽点" */
  const saved = computed(() =>
    items.value.reduce((sum, i) => sum + Math.max(0, i.originalPrice - i.price) * i.qty, 0),
  )

  function upsert(item: CartItem): void {
    const idx = items.value.findIndex((i) => i.productId === item.productId)
    if (idx >= 0) items.value[idx] = item
    else items.value.push(item)
  }

  async function load(): Promise<void> {
    loading.value = true
    try {
      items.value = (await cartApi.list()) ?? []
      badgeCount.value = items.value.length
    } finally {
      loading.value = false
    }
  }

  /** 只取条目数：一次 HLen，不必为了角标把两个 Hash 全量拉回来 */
  async function loadCount(): Promise<void> {
    try {
      const res = await cartApi.count()
      badgeCount.value = res?.count ?? 0
    } catch {
      /* 角标是装饰性信息，失败就保持原值，不打扰用户 */
    }
  }

  async function add(productId: number, qty = 1): Promise<CartItem | undefined> {
    busyId.value = productId
    try {
      const item = await cartApi.add(productId, qty)
      if (item) upsert(item)
      return item
    } finally {
      busyId.value = null
      // 台账已变，角标跟着刷新（一次 HLen，成本可忽略）
      void loadCount()
    }
  }

  /** qty <= 0 表示删掉这一行 */
  async function setQty(productId: number, qty: number): Promise<void> {
    busyId.value = productId
    try {
      if (qty <= 0) {
        await cartApi.remove(productId)
        items.value = items.value.filter((i) => i.productId !== productId)
        return
      }
      const item = await cartApi.update(productId, qty)
      if (item) upsert(item)
    } finally {
      busyId.value = null
      // 台账已变，角标跟着刷新（一次 HLen，成本可忽略）
      void loadCount()
    }
  }

  async function remove(productId: number): Promise<void> {
    busyId.value = productId
    try {
      await cartApi.remove(productId)
      items.value = items.value.filter((i) => i.productId !== productId)
    } finally {
      busyId.value = null
      // 台账已变，角标跟着刷新（一次 HLen，成本可忽略）
      void loadCount()
    }
  }

  async function clear(): Promise<void> {
    await cartApi.clear()
    items.value = []
    badgeCount.value = 0
  }

  /** 登出时清掉本地镜像（Redis 侧数据保留，重新登录仍在） */
  function reset(): void {
    items.value = []
    busyId.value = null
    badgeCount.value = 0
  }

  return {
    items,
    loading,
    busyId,
    count,
    badgeCount,
    subtotal,
    saved,
    load,
    loadCount,
    add,
    setQty,
    remove,
    clear,
    reset,
  }
})
