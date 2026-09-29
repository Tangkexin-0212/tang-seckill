<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTags, type TagItem } from '@/stores/tags'

const route = useRoute()
const router = useRouter()
const tags = useTags()

const current = computed(() => route.path)

function go(tag: TagItem): void {
  if (tag.path !== current.value) void router.push(tag.path)
}

function close(tag: TagItem): void {
  const closed = tags.close(tag.key)
  if (!closed) return
  // 关掉的正好是当前页 → 退到剩下的最后一个标签，而不是甩回首页
  if (closed.path === current.value) {
    const last = tags.visited[tags.visited.length - 1]
    void router.push(last ? last.path : '/dashboard')
  }
}

function closeOthers(): void {
  tags.closeOthers(current.value)
}

function closeAll(): void {
  tags.closeAll()
  void router.push('/dashboard')
}
</script>

<template>
  <div class="tags">
    <el-scrollbar class="tags__scroll">
      <div class="tags__list">
        <div
          v-for="t in tags.visited"
          :key="t.key"
          class="tag"
          :class="{ 'is-active': t.path === current }"
          @click="go(t)"
        >
          <span v-if="t.path === current" class="tag__dot" />
          <span class="tag__text">{{ t.title }}</span>
          <el-icon v-if="!t.affix" class="tag__close" @click.stop="close(t)"><Close /></el-icon>
        </div>
      </div>
    </el-scrollbar>

    <el-dropdown trigger="click" placement="bottom-end">
      <button class="tags__more" type="button" title="标签操作">
        <el-icon><ArrowDown /></el-icon>
      </button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item @click="closeOthers">关闭其他</el-dropdown-item>
          <el-dropdown-item @click="closeAll">关闭全部</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<style scoped>
.tags {
  flex: none;
  display: flex;
  align-items: center;
  height: var(--tags-h);
  padding: 0 10px 0 6px;
  background: #fff;
  border-bottom: 1px solid var(--line);
}

.tags__scroll {
  flex: 1;
  min-width: 0;
}

.tags__list {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 4px;
  white-space: nowrap;
}

.tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 9px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--bg-page);
  color: var(--ink-600);
  font-size: 12px;
  cursor: pointer;
  user-select: none;
  transition:
    color var(--t-fast) var(--ease),
    border-color var(--t-fast) var(--ease),
    background var(--t-fast) var(--ease);
}

.tag:hover {
  color: var(--brand-600);
  border-color: var(--brand-200);
}

.tag.is-active {
  background: #fff;
  color: var(--brand-600);
  border-color: var(--brand-600);
  font-weight: 500;
}

.tag__dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--green-600);
}

.tag__close {
  font-size: 12px;
  color: var(--ink-400);
  border-radius: 50%;
  transition:
    background var(--t-fast) var(--ease),
    color var(--t-fast) var(--ease);
}

.tag__close:hover {
  background: var(--brand-600);
  color: #fff;
}

.tags__more {
  flex: none;
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: #fff;
  color: var(--ink-500);
  cursor: pointer;
}

.tags__more:hover {
  color: var(--brand-600);
  border-color: var(--brand-200);
}
</style>
