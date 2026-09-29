<script setup lang="ts">
import { computed, ref, watch } from 'vue'

/**
 * 商品缩略图位。
 *
 * 为什么需要它：示例数据里 cover 大量是 "https://example.com/xxx.png"、空串或 "x"，
 * 直接 <img> 会满屏破图。这里只在 URL 合法时真加载，失败即降级为程序化占位牌
 * （首字 + 同色系浅底），保证界面上永远不会出现破图。
 */
const props = withDefaults(
  defineProps<{
    src?: string
    name?: string
    /** 尺寸：表格里用 sm，卡片/详情用 lg */
    size?: 'sm' | 'md' | 'lg'
    /** 圆形（用于用户/图标位） */
    circle?: boolean
  }>(),
  { src: '', name: '', size: 'md', circle: false },
)

const failed = ref(false)
watch(
  () => props.src,
  () => {
    failed.value = false
  },
)

const usable = computed(() => !!props.src && /^(https?:)?\/\//.test(props.src) && !failed.value)

const initial = computed(() => {
  const n = (props.name || '').trim()
  return n ? n.slice(0, 1).toUpperCase() : '#'
})

/** 占位色只在这三种品牌色里取，绝不让脏数据生成一个野色 */
const TONES = [
  { bg: 'linear-gradient(135deg,#e9f1ff,#f6faff)', fg: '#1677ff' },
  { bg: 'linear-gradient(135deg,#e7f7ef,#f4fdf8)', fg: '#18a058' },
  { bg: 'linear-gradient(135deg,#eef2f7,#f9fbfd)', fg: '#5a6270' },
] as const

const tone = computed(() => {
  const key = props.name || String(props.src)
  let h = 0
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) % 9973
  return TONES[h % TONES.length]
})
</script>

<template>
  <span
    class="plate"
    :class="[`plate--${size}`, { 'plate--circle': circle }]"
    :style="{ background: tone.bg, color: tone.fg }"
  >
    <img v-if="usable" class="plate__img" :src="src" :alt="name" loading="lazy" @error="failed = true" />
    <span v-else class="plate__initial">{{ initial }}</span>
  </span>
</template>

<style scoped>
.plate {
  display: inline-grid;
  place-items: center;
  flex: none;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  font-weight: 600;
  user-select: none;
}

.plate--circle {
  border-radius: 50%;
}

.plate--sm {
  width: 34px;
  height: 34px;
  font-size: 14px;
}

.plate--md {
  width: 48px;
  height: 48px;
  font-size: 18px;
}

.plate--lg {
  width: 100%;
  aspect-ratio: 4 / 3;
  font-size: 44px;
}

.plate__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.plate__initial {
  line-height: 1;
}
</style>
