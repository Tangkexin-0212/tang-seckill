<script setup lang="ts">
import { computed } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import NavBar from '@/layouts/components/NavBar.vue'
import SideMenu from '@/layouts/components/SideMenu.vue'
import TagsView from '@/layouts/components/TagsView.vue'
import { useApp } from '@/stores/app'
import { useTags } from '@/stores/tags'

const app = useApp()
const tags = useTags()
const route = useRoute()

const asideWidth = computed(() => (app.collapsed ? 'var(--aside-w-collapsed)' : 'var(--aside-w)'))

// 菜单/标签页跟着路由走：每次进入页面就登记一个标签
tags.ensure(route)
</script>

<template>
  <div class="layout">
    <aside class="layout__aside" :style="{ width: asideWidth }">
      <SideMenu />
    </aside>

    <div class="layout__body">
      <NavBar />
      <TagsView />

      <main class="layout__main">
        <RouterView v-slot="{ Component }">
          <Transition name="page" mode="out-in">
            <!-- key 里带上 refreshKey：顶栏"刷新"按钮靠它强制重建当前页组件 -->
            <component :is="Component" :key="app.refreshKey" />
          </Transition>
        </RouterView>

        <footer class="layout__foot">
          Powered by 秒杀管理台 · Kitex + Hertz + Redis Lua · Redis 为权威库存，MySQL 最终一致
        </footer>
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
  background: var(--bg-page);
}

.layout__aside {
  flex: none;
  height: 100%;
  background: #fff;
  border-right: 1px solid var(--line);
  transition: width var(--t-mid) var(--ease);
  overflow: hidden;
  z-index: 20;
}

.layout__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.layout__main {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
}

.layout__foot {
  margin-top: auto;
  padding: 18px 16px;
  text-align: center;
  font-size: 12px;
  color: var(--ink-400);
  border-top: 1px solid var(--line-2);
  background: #fff;
}
</style>
