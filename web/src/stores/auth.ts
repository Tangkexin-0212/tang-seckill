import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { userApi } from '@/api'
import { useServerNow } from '@/composables/useServerClock'

const STORAGE_KEY = 'sk.auth.v1'

interface PersistedAuth {
  token: string
  uid: number
  username: string
  expireAtMs: number
}

/**
 * 后端没有 refresh token 接口（token 固定 24h），所以过期只能重新登录。
 * 这里在读取时就顺手清掉已过期的存档，避免带着死 token 发请求再收 10005。
 */
function loadPersisted(): PersistedAuth | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const p = JSON.parse(raw) as PersistedAuth
    if (!p?.token || !p?.expireAtMs) return null
    if (p.expireAtMs <= Date.now()) {
      localStorage.removeItem(STORAGE_KEY)
      return null
    }
    return p
  } catch {
    localStorage.removeItem(STORAGE_KEY)
    return null
  }
}

export const useAuth = defineStore('auth', () => {
  const boot = loadPersisted()
  const now = useServerNow()

  const token = ref(boot?.token ?? '')
  const uid = ref(boot?.uid ?? 0)
  const username = ref(boot?.username ?? '')
  const expireAtMs = ref(boot?.expireAtMs ?? 0)

  // 依赖共享时钟，所以 token 到点会自动变 false，不需要额外定时器
  const isLoggedIn = computed(() => token.value !== '' && expireAtMs.value > now.value)
  const msLeft = computed(() => Math.max(0, expireAtMs.value - now.value))

  function persist(): void {
    const payload: PersistedAuth = {
      token: token.value,
      uid: uid.value,
      username: username.value,
      expireAtMs: expireAtMs.value,
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
  }

  function clear(): void {
    token.value = ''
    uid.value = 0
    username.value = ''
    expireAtMs.value = 0
    localStorage.removeItem(STORAGE_KEY)
  }

  async function login(name: string, password: string): Promise<void> {
    const res = await userApi.login(name, password)
    token.value = res.token
    uid.value = res.uid
    username.value = name
    expireAtMs.value = res.expireAtMs || Date.now() + 24 * 3600 * 1000
    persist()
  }

  async function register(name: string, password: string): Promise<number> {
    const res = await userApi.register(name, password)
    return res.uid
  }

  return { token, uid, username, expireAtMs, isLoggedIn, msLeft, login, register, clear, persist }
})
