<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { MENUS } from '@/layouts/menu'
import { useApp } from '@/stores/app'
import { useCart } from '@/stores/cart'

const app = useApp()
const cart = useCart()
const route = useRoute()

/**
 * 高亮项要对齐"叶子路由"：
 * /seckill/grab/23 是隐藏路由（有页面但不在菜单里），
 * 此时应该让它的入口 /seckill/activity 保持高亮，否则用户会以为菜单丢了。
 */
const activeIndex = computed(() => {
  const p = route.path
  if (p.startsWith('/seckill/grab')) return '/seckill/activity'
  if (p.startsWith('/seckill')) return '/seckill/activity'
  return p
})

const groupActive = computed(() => activeIndex.value.startsWith('/seckill'))

/** 角标用条目数（HLen 口径），与 /api/cart/count 一致 */
const cartCount = computed(() => cart.badgeCount)
</script>

<template>
  <div class="side" :class="{ 'side--collapsed': app.collapsed }">
    <!-- 品牌区：蓝绿渐变标 + 名称 -->
    <div class="side__brand">
      <span class="side__logo">
        <svg viewBox="0 0 32 32" width="26" height="26" aria-hidden="true">
          <defs>
            <linearGradient id="brandGrad" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" stop-color="#1677ff" />
              <stop offset="1" stop-color="#18a058" />
            </linearGradient>
          </defs>
          <rect x="1" y="1" width="30" height="30" rx="8" fill="url(#brandGrad)" />
          <path d="M13 8h6l-2.2 6H20l-8 10 2.2-7H10z" fill="#fff" />
        </svg>
      </span>
      <span v-show="!app.collapsed" class="side__brand-text">
        <span class="side__brand-title">秒杀管理台</span>
        <span class="side__brand-sub">SECKILL ADMIN</span>
      </span>
    </div>

    <el-scrollbar class="side__scroll">
      <el-menu
        :default-active="activeIndex"
        :collapse="app.collapsed"
        :collapse-transition="false"
        unique-opened
        router
        class="side__menu"
        :class="{ 'side__menu--expanded': !app.collapsed }"
      >
        <template v-for="m in MENUS" :key="m.title">
          <el-sub-menu v-if="m.children?.length" :index="m.title">
            <template #title>
              <el-icon><component :is="m.icon" /></el-icon>
              <span>{{ m.title }}</span>
            </template>
            <el-menu-item
              v-for="c in m.children"
              :key="c.path"
              :index="c.path"
              :class="{ 'is-group-active': groupActive }"
            >
              <el-icon><component :is="c.icon" /></el-icon>
              <template #title>{{ c.title }}</template>
            </el-menu-item>
          </el-sub-menu>

          <el-menu-item v-else :index="m.path">
            <el-icon><component :is="m.icon" /></el-icon>
            <template #title>
              <span class="side__label">{{ m.title }}</span>
              <el-badge
                v-if="m.path === '/cart/list' && cartCount > 0"
                :value="cartCount"
                class="side__badge"
              />
            </template>
          </el-menu-item>
        </template>
      </el-menu>
    </el-scrollbar>

    <!-- 底部收起开关（gin-vue-admin 同款位置） -->
    <div class="side__foot" @click="app.toggle()">
      <el-icon><component :is="app.collapsed ? 'Expand' : 'Fold'" /></el-icon>
      <span v-show="!app.collapsed">收起导航</span>
    </div>
  </div>
</template>

<style scoped>
.side {
  display: flex;
  flex-direction: column;
  height: 100%;
}

/* ---- 品牌区 ---- */
.side__brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: var(--navbar-h);
  padding: 0 16px;
  border-bottom: 1px solid var(--line-2);
  flex: none;
}

.side--collapsed .side__brand {
  padding: 0;
  justify-content: center;
}

.side__logo {
  display: grid;
  place-items: center;
  flex: none;
}

.side__brand-text {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
  overflow: hidden;
}

.side__brand-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--ink-900);
  letter-spacing: 0.01em;
}

.side__brand-sub {
  font-family: var(--font-mono);
  font-size: 9px;
  letter-spacing: 0.14em;
  color: var(--green-600);
}

.side__scroll {
  flex: 1;
  min-height: 0;
  padding: 8px 0;
}

/* ---- 菜单：居中实底蓝绿渐变 = 当前页 ---- */
.side__menu {
  border-right: 0;
  background: transparent;
}

.side__menu :deep(.el-menu-item),
.side__menu :deep(.el-sub-menu__title) {
  height: 42px;
  line-height: 42px;
  color: var(--ink-700);
  font-size: 14px;
}

.side__menu--expanded :deep(.el-menu-item),
.side__menu--expanded :deep(.el-sub-menu__title) {
  margin: 2px 8px;
  border-radius: var(--radius);
  width: auto;
}

.side__menu :deep(.el-menu-item:hover),
.side__menu :deep(.el-sub-menu__title:hover) {
  background: var(--bg-hover);
  color: var(--brand-600);
}

.side__menu :deep(.el-menu-item.is-active) {
  background: linear-gradient(90deg, var(--brand-600), var(--green-600));
  color: #fff;
  font-weight: 500;
  box-shadow: 0 2px 10px rgba(22, 119, 255, 0.26);
}

.side__menu :deep(.el-menu-item.is-active:hover) {
  background: linear-gradient(90deg, var(--brand-500), var(--green-500));
  color: #fff;
}

.side__menu :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: var(--brand-600);
  font-weight: 500;
}

.side__menu :deep(.el-sub-menu .el-menu-item) {
  min-width: auto;
}

.side__label {
  flex: 1;
}

.side__badge {
  margin-left: 6px;
}

.side__badge :deep(.el-badge__content) {
  border: 0;
  background: var(--green-600);
}

/* ---- 底部开关 ---- */
.side__foot {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 44px;
  border-top: 1px solid var(--line-2);
  color: var(--ink-500);
  font-size: 13px;
  cursor: pointer;
  user-select: none;
  transition: background var(--t-fast) var(--ease);
}

.side__foot:hover {
  background: var(--bg-hover);
  color: var(--brand-600);
}
</style>
