<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { flatMenus } from '@/layouts/menu'
import { useApp } from '@/stores/app'
import { useAuth } from '@/stores/auth'
import { useCart } from '@/stores/cart'
import { useTags } from '@/stores/tags'
import { feedback } from '@/utils/feedback'
import { duration } from '@/utils/format'

const app = useApp()
const auth = useAuth()
const cart = useCart()
const tags = useTags()
const route = useRoute()
const router = useRouter()

/* ---- 面包屑：只显示有 title 的层级，AdminLayout 本身不参与 ---- */
const crumbs = computed(() =>
  route.matched
    .filter((r) => r.meta?.title && r.meta.hidden !== true)
    .map((r) => ({ title: r.meta.title as string })),
)

/* ---- 菜单搜索 ---- */
const searchVisible = ref(false)
const keyword = ref('')
const candidates = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const all = flatMenus()
  if (!kw) return all
  return all.filter((m) => m.title.toLowerCase().includes(kw) || m.group.toLowerCase().includes(kw))
})

function openSearch(): void {
  keyword.value = ''
  searchVisible.value = true
}

function jump(path: string): void {
  searchVisible.value = false
  void router.push(path)
}

/* ---- 全屏 ---- */
const isFull = ref(false)

function syncFull(): void {
  isFull.value = !!document.fullscreenElement
}

async function toggleFull(): Promise<void> {
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await document.documentElement.requestFullscreen()
  } catch {
    feedback.warning('当前浏览器不允许全屏', '')
  }
}

onMounted(() => document.addEventListener('fullscreenchange', syncFull))
onBeforeUnmount(() => document.removeEventListener('fullscreenchange', syncFull))

/* ---- 用户 ---- */
function logout(): void {
  auth.clear()
  tags.reset()
  feedback.success('已退出登录')
  void router.push('/login')
}

function gotoLogin(): void {
  void router.push({ name: 'login', query: { redirect: route.fullPath } })
}
</script>

<template>
  <header class="navbar">
    <button class="iconbtn" type="button" :title="app.collapsed ? '展开导航' : '收起导航'" @click="app.toggle()">
      <el-icon><component :is="app.collapsed ? 'Expand' : 'Fold'" /></el-icon>
    </button>

    <button class="iconbtn" type="button" title="刷新当前页" @click="app.refresh()">
      <el-icon><Refresh /></el-icon>
    </button>

    <el-breadcrumb class="navbar__crumb" separator="/">
      <el-breadcrumb-item v-for="(c, i) in crumbs" :key="i">{{ c.title }}</el-breadcrumb-item>
    </el-breadcrumb>

    <div class="navbar__fill" />

    <span class="pill">
      <el-icon><Connection /></el-icon>
      库存权威：Redis
    </span>

    <button class="iconbtn" type="button" title="菜单搜索" @click="openSearch">
      <el-icon><Search /></el-icon>
    </button>

    <button class="iconbtn" type="button" :title="isFull ? '退出全屏' : '全屏'" @click="toggleFull">
      <el-icon><FullScreen /></el-icon>
    </button>

    <el-popover placement="bottom-end" :width="320" trigger="click">
      <template #reference>
        <button class="iconbtn" type="button" title="系统信息">
          <el-icon><InfoFilled /></el-icon>
        </button>
      </template>
      <div class="info">
        <p class="info__title">链路口径</p>
        <ul class="info__list">
          <li><span>网关</span><b class="mono">127.0.0.1:8080</b></li>
          <li><span>鉴权</span><b class="mono">Authorization: Bearer</b></li>
          <li><span>购物车</span><b class="mono">Redis Hash · {{ cart.badgeCount }} 种</b></li>
          <li><span>库存</span><b>Redis 权威，MySQL 最终一致</b></li>
        </ul>
        <p class="info__note">
          下单写 order 库后由 worker 异步消费 Stream 落库，所以后台报表与 Redis 快照之间有几百毫秒的可见延迟。
        </p>
      </div>
    </el-popover>

    <el-dropdown v-if="auth.isLoggedIn" trigger="click" placement="bottom-end">
      <div class="user">
        <span class="user__avatar">{{ (auth.username || 'U').slice(0, 1).toUpperCase() }}</span>
        <span class="user__meta">
          <span class="user__name">{{ auth.username || `UID ${auth.uid}` }}</span>
          <span class="user__sub mono">号牌剩余 {{ duration(auth.msLeft) }}</span>
        </span>
        <el-icon class="user__caret"><ArrowDown /></el-icon>
      </div>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item disabled>UID {{ auth.uid }}</el-dropdown-item>
          <el-dropdown-item divided @click="router.push('/order/list')">我的订单</el-dropdown-item>
          <el-dropdown-item @click="router.push('/cart/list')">购物车</el-dropdown-item>
          <el-dropdown-item divided @click="logout">退出登录</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>

    <el-button v-else type="primary" size="small" @click="gotoLogin">登录</el-button>

    <!-- 菜单搜索 -->
    <el-dialog v-model="searchVisible" title="菜单搜索" width="460px" append-to-body>
      <el-input v-model="keyword" placeholder="输入菜单名称，例如：订单" clearable autofocus>
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <div class="find">
        <p v-if="candidates.length === 0" class="find__empty">没有匹配的菜单</p>
        <button v-for="m in candidates" :key="m.path" class="find__item" type="button" @click="jump(m.path)">
          <el-icon><component :is="m.icon" /></el-icon>
          <span class="find__title">{{ m.title }}</span>
          <span v-if="m.group" class="find__group">{{ m.group }}</span>
          <span class="find__path mono">{{ m.path }}</span>
        </button>
      </div>
    </el-dialog>
  </header>
</template>

<style scoped>
.navbar {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  height: var(--navbar-h);
  padding: 0 14px 0 8px;
  background: #fff;
  border-bottom: 1px solid var(--line);
}

.iconbtn {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--ink-600);
  font-size: 16px;
  cursor: pointer;
  transition:
    background var(--t-fast) var(--ease),
    color var(--t-fast) var(--ease);
}

.iconbtn:hover {
  background: var(--bg-hover);
  color: var(--brand-600);
}

.navbar__crumb {
  margin-left: 8px;
  font-size: 13px;
}

.navbar__crumb :deep(.el-breadcrumb__inner) {
  color: var(--ink-500);
  font-weight: 400;
}

.navbar__crumb :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--ink-900);
  font-weight: 500;
}

.navbar__fill {
  flex: 1;
}

/* 一枚信息胶囊：告诉使用者这套系统的库存口径，而不是放一个无意义的装饰徽标 */
.pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 10px;
  margin-right: 4px;
  border: 1px solid var(--brand-200);
  border-radius: 14px;
  background: var(--brand-50);
  color: var(--brand-700);
  font-size: 12px;
}

/* ---- 用户 ---- */
.user {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 8px 3px 3px;
  border-radius: 20px;
  cursor: pointer;
  transition: background var(--t-fast) var(--ease);
}

.user:hover {
  background: var(--bg-hover);
}

.user__avatar {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--brand-600), var(--green-600));
  color: #fff;
  font-size: 13px;
  font-weight: 600;
}

.user__meta {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
}

.user__name {
  font-size: 13px;
  color: var(--ink-900);
}

.user__sub {
  font-size: 10px;
  color: var(--ink-400);
}

.user__caret {
  font-size: 12px;
  color: var(--ink-400);
}

/* ---- 信息面板 ---- */
.info__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink-900);
  margin-bottom: 8px;
}

.info__list {
  list-style: none;
  padding: 0;
  margin: 0 0 8px;
  font-size: 12px;
}

.info__list li {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 0;
  border-bottom: 1px dashed var(--line-2);
}

.info__list span {
  color: var(--ink-500);
}

.info__list b {
  color: var(--ink-900);
  font-weight: 500;
}

.info__note {
  font-size: 12px;
  line-height: 1.7;
  color: var(--ink-500);
}

/* ---- 菜单搜索 ---- */
.find {
  margin-top: 12px;
  max-height: 320px;
  overflow: auto;
}

.find__empty {
  padding: 24px 0;
  text-align: center;
  color: var(--ink-400);
  font-size: 13px;
}

.find__item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 9px 10px;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  text-align: left;
  font-size: 13px;
  color: var(--ink-700);
  cursor: pointer;
}

.find__item:hover {
  background: var(--bg-hover);
  color: var(--brand-600);
}

.find__title {
  font-weight: 500;
}

.find__group {
  font-size: 11px;
  color: var(--ink-400);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 0 7px;
}

.find__path {
  margin-left: auto;
  font-size: 11px;
  color: var(--ink-400);
}

@media (max-width: 900px) {
  .pill,
  .user__meta {
    display: none;
  }
}
</style>
