<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { seckillApi } from '@/api'
import { ApiError, CODE, netStats } from '@/api/client'
import type { SeckillInfo } from '@/api/types'
import CoverPlate from '@/components/CoverPlate.vue'
import { recordLatency, useServerNow } from '@/composables/useServerClock'
import { useAuth } from '@/stores/auth'
import { feedback } from '@/utils/feedback'
import { dateTime, discountLabel, duration, money } from '@/utils/format'
import { activityPhase, phaseMeta, stockColor, stockPercent } from '@/utils/status'

const props = defineProps<{ id: string }>()

const router = useRouter()
const auth = useAuth()
const now = useServerNow()

const activityId = computed(() => Number(props.id))
const info = ref<SeckillInfo | null>(null)
const loading = ref(true)
const fatal = ref('')

/**
 * 出价状态机。刻意把"业务上不许你抢"的几种情况和"真出错"分开：
 * 前者（抢过了/售罄了/被限流）是这个产品的正常剧情，不该显示成系统报错。
 */
type GrabState = 'idle' | 'pending' | 'won' | 'already' | 'soldout' | 'limited' | 'error'
const state = ref<GrabState>('idle')
const orderNo = ref('')
const cooldownUntil = ref(0)

const resultEl = ref<HTMLElement | null>(null)

/** silent=true 时不动 loading，避免抢购后的结果卡片被骨架闪掉 */
async function load(silent = false): Promise<void> {
  if (!silent) loading.value = true
  fatal.value = ''
  try {
    const res = await seckillApi.info(activityId.value)
    info.value = res
    recordLatency(netStats.lastRttMs)
  } catch (err) {
    const e = err instanceof ApiError ? err : new ApiError(0, String(err))
    if (e.code === CODE.ACTIVITY_INVALID) fatal.value = '活动不存在、未开始或已结束。'
    else {
      fatal.value = e.message || '加载活动信息失败。'
      feedback.fromError(err)
    }
  } finally {
    if (!silent) loading.value = false
  }
}

onMounted(load)

watch(activityId, () => {
  state.value = 'idle'
  orderNo.value = ''
  info.value = null
  void load()
})

/* ---------------- 派生状态 ---------------- */
const phase = computed(() =>
  info.value ? activityPhase(info.value.startTimeMs, info.value.endTimeMs, info.value.status, now.value) : 'ended',
)

const soldOut = computed(() => !!info.value && (info.value.soldOut || info.value.remainStock <= 0))

const targetMs = computed(() => {
  if (!info.value) return 0
  return phase.value === 'upcoming' ? info.value.startTimeMs : info.value.endTimeMs
})

const remainMs = computed(() => targetMs.value - now.value)

const countdownLabel = computed(() =>
  phase.value === 'upcoming' ? '距开槌' : phase.value === 'live' ? '距落槌' : '本场已落槌',
)

/**
 * 倒计时跨过 0 的那一刻自动同步一次权威状态。
 * 用一个普通闭包变量做闸门，避免共享时钟每 500ms 触发一次重复请求。
 */
let crossed = false
watch(targetMs, () => {
  crossed = false
})
watch(remainMs, (left) => {
  if (left <= 0 && !crossed) {
    crossed = true
    void load(true)
  }
})

const cooldownLeft = computed(() => Math.max(0, Math.ceil((cooldownUntil.value - now.value) / 1000)))
const latency = computed(() => Math.round(netStats.lastRttMs))

const settled = computed(() => state.value === 'won' || state.value === 'already')

const ctaLabel = computed(() => {
  if (state.value === 'pending') return '正在击穿库存…'
  if (cooldownLeft.value > 0) return `限流冷却 ${cooldownLeft.value}s`
  if (settled.value) return '查看订单'
  if (phase.value === 'upcoming') return '尚未开槌'
  if (phase.value === 'ended') return '本场已落槌'
  if (soldOut.value) return '已售罄'
  if (state.value === 'error') return '重试出价'
  return '立即抢购'
})

const ctaDisabled = computed(
  () =>
    state.value === 'pending' ||
    cooldownLeft.value > 0 ||
    phase.value !== 'live' ||
    soldOut.value ||
    !info.value ||
    !!fatal.value,
)

const hint = computed(() => {
  if (!auth.isLoggedIn) return '抢购需要登录，点击后会先带你登录再回到本页。'
  if (phase.value === 'upcoming') return '钟声未响，开槌后按钮会自动可用。'
  if (phase.value === 'ended') return '本场已经落槌。'
  if (soldOut.value) return '库存见底，可以等有人放弃支付后再刷新。'
  if (cooldownLeft.value > 0) return '三层令牌桶在保护后端，冷却结束会自动恢复。'
  return '每人限购 1 件。重复点击不会重复扣库存，后端会直接告诉你原先的订单号。'
})

/* ---------------- 抢购 ---------------- */
async function onGrab(): Promise<void> {
  if (settled.value) {
    void router.push('/order/list')
    return
  }
  if (!auth.isLoggedIn) {
    void router.push({ name: 'login', query: { redirect: `/seckill/grab/${activityId.value}` } })
    return
  }

  state.value = 'pending'
  try {
    const res = await seckillApi.grab(activityId.value)
    state.value = 'won'
    orderNo.value = res.orderNo
    feedback.success('落槌！', `订单号 ${res.orderNo}`)
  } catch (err) {
    const e = err instanceof ApiError ? err : new ApiError(0, String(err))
    switch (e.code) {
      case CODE.ALREADY_BOUGHT: {
        // 幂等命中不是失败：后端把原订单号放在 msg 里，直接给支付入口
        state.value = 'already'
        const m = e.message.match(/SK[0-9a-zA-Z]+/)
        orderNo.value = m ? m[0] : ''
        feedback.info('你已经抢到过这一件', orderNo.value ? `订单号 ${orderNo.value}` : '每人限购 1 件')
        break
      }
      case CODE.SOLD_OUT:
        state.value = 'soldout'
        feedback.warning('已售罄', '库存被抢空了')
        break
      case CODE.RATE_LIMITED:
        state.value = 'idle'
        cooldownUntil.value = now.value + 4000
        feedback.warning('被限流了', '令牌桶见底，冷却 4 秒后可再试')
        break
      case CODE.ACTIVITY_INVALID:
        state.value = 'error'
        fatal.value = '活动不在可参与状态。'
        break
      default:
        state.value = 'error'
        feedback.fromError(err)
        break
    }
  } finally {
    void load(true)
  }
}

/** 结果出现在首屏之下时主动带进视野，否则用户只会看到按钮文字变了 */
watch(state, async (v) => {
  if (v !== 'won' && v !== 'already' && v !== 'soldout') return
  await nextTick()
  resultEl.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
})

async function release(): Promise<void> {
  const ok = await feedback.confirm('释放资格会把库存立刻回补给其他人，且当前订单作废。确定继续？', '释放资格', '确定释放')
  if (!ok) return
  try {
    const res = await seckillApi.cancel(activityId.value)
    if (res.released) feedback.success('资格已释放', '库存已回补，可以重新抢购')
    else feedback.warning('没有可释放的资格')
    state.value = 'idle'
    orderNo.value = ''
  } catch (err) {
    feedback.fromError(err)
  } finally {
    void load(true)
  }
}
</script>

<template>
  <div class="app-page">
    <!-- ============ 页头 ============ -->
    <div class="card">
      <div class="card__hd">
        <el-button link type="primary" @click="router.push('/seckill/activity')">
          <el-icon><ArrowLeft /></el-icon>返回活动列表
        </el-button>
        <span class="card__title" style="margin-left: 6px">
          抢购台
          <span class="mono lot">LOT {{ String(activityId).padStart(3, '0') }}</span>
        </span>
        <div class="card__extra">
          <el-tag v-if="info" :type="phaseMeta(phase).type" effect="light">{{ phaseMeta(phase).label }}</el-tag>
          <el-button :loading="loading" @click="load(true)"><el-icon><Refresh /></el-icon>刷新</el-button>
        </div>
      </div>
    </div>

    <!-- ============ 主体 ============ -->
    <el-row v-if="info" :gutter="12" class="mt">
      <el-col :xs="24" :lg="15">
        <!-- 拍品 -->
        <div class="card">
          <div class="card__hd"><span class="card__title">拍品</span></div>
          <div class="card__bd lot-body">
            <CoverPlate class="lot-cover" :src="info.cover" :name="info.productName" size="lg" />
            <div class="lot-info">
              <h2 class="lot-name">{{ info.productName || '未命名拍品' }}</h2>
              <div class="lot-price">
                <span class="mono now">{{ money(info.seckillPrice) }}</span>
                <span class="mono text-strike was">{{ money(info.originalPrice) }}</span>
                <el-tag v-if="discountLabel(info.seckillPrice, info.originalPrice)" type="primary" effect="plain" size="small">
                  {{ discountLabel(info.seckillPrice, info.originalPrice) }}
                </el-tag>
              </div>
              <el-descriptions :column="2" size="small" border class="lot-desc">
                <el-descriptions-item label="商品 ID">
                  <span class="mono">#{{ info.productId }}</span>
                </el-descriptions-item>
                <el-descriptions-item label="总库存">
                  <span class="mono">{{ info.totalStock }}</span>
                </el-descriptions-item>
              </el-descriptions>
            </div>
          </div>
        </div>

        <!-- 抢购 -->
        <div class="card">
          <div class="card__hd">
            <span class="card__title">抢购</span>
            <div class="card__extra">
              <span class="text-soft">本次往返 {{ latency }}ms</span>
            </div>
          </div>
          <div class="card__bd">
            <div class="dial">
              <div class="dial__caption">{{ countdownLabel }}</div>
              <div class="dial__value mono" :class="{ 'is-dead': phase === 'ended' || soldOut }">
                {{ duration(Math.max(0, remainMs)) }}
              </div>
              <el-progress
                v-if="info.startTimeMs > 0"
                :percentage="
                  Math.max(
                    0,
                    Math.min(
                      100,
                      Math.round(
                        ((targetMs - info.startTimeMs - Math.max(0, remainMs)) / (targetMs - info.startTimeMs)) * 100,
                      ),
                    ),
                  )
                "
                :stroke-width="4"
                :show-text="false"
                color="#1677ff"
              />
            </div>

            <div class="stock">
              <div class="stock__head">
                <span class="text-soft">剩余库存</span>
                <span class="mono">
                  <b :class="{ 'text-danger': soldOut }">{{ info.remainStock }}</b> / {{ info.totalStock }}
                </span>
              </div>
              <el-progress
                :percentage="stockPercent(info.remainStock, info.totalStock)"
                :stroke-width="12"
                :color="stockColor(info.remainStock, info.totalStock)"
                :format="() => `${info?.remainStock ?? 0} 件`"
              />
            </div>

            <el-button
              class="cta"
              :type="phase === 'live' && !soldOut && !settled ? 'primary' : 'default'"
              size="large"
              :loading="state === 'pending'"
              :disabled="ctaDisabled"
              @click="onGrab"
            >
              <el-icon v-if="!settled && phase === 'live'"><Pointer /></el-icon>{{ ctaLabel }}
            </el-button>

            <el-alert class="mt-sm" type="info" :closable="false" show-icon :title="hint" />

            <!-- 结果 -->
            <div v-if="state === 'won' || state === 'already' || state === 'soldout'" ref="resultEl" class="result">
              <el-result
                :icon="state === 'soldout' ? 'warning' : 'success'"
                :title="state === 'won' ? '落槌！这一件归你了' : state === 'already' ? '你之前已经拿下这一件' : '这一件已被抢空'"
                :sub-title="
                  state === 'soldout'
                    ? '如果有人放弃支付，库存会回到 Redis，刷新即可看到'
                    : `订单号 ${orderNo || '——'}`
                "
              >
                <template #extra>
                  <template v-if="state !== 'soldout'">
                    <el-button type="primary" @click="router.push('/order/list')">
                      <el-icon><Money /></el-icon>去支付
                    </el-button>
                    <el-button type="danger" plain @click="release">释放资格</el-button>
                  </template>
                  <template v-else>
                    <el-button @click="load(true)">
                      <el-icon><Refresh /></el-icon>刷新余量
                    </el-button>
                    <el-button type="primary" plain @click="router.push('/seckill/activity')">换一场</el-button>
                  </template>
                </template>
              </el-result>
            </div>
          </div>
        </div>
      </el-col>

      <el-col :xs="24" :lg="9">
        <div class="card">
          <div class="card__hd"><span class="card__title">活动信息</span></div>
          <div class="card__bd">
            <el-descriptions :column="1" border size="small">
              <el-descriptions-item label="活动 ID">
                <span class="mono">#{{ info.activityId }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="开拍时间">
                <span class="mono">{{ dateTime(info.startTimeMs) }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="落槌时间">
                <span class="mono">{{ dateTime(info.endTimeMs) }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="剩余时长">
                <span class="mono">{{ duration(Math.max(0, info.endTimeMs - now)) }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="去化率">
                <span class="mono">{{ 100 - stockPercent(info.remainStock, info.totalStock) }}%</span>
              </el-descriptions-item>
              <el-descriptions-item label="售罄标记">
                <el-tag :type="info.soldOut ? 'danger' : 'success'" size="small">
                  {{ info.soldOut ? '已售罄' : '可抢购' }}
                </el-tag>
              </el-descriptions-item>
            </el-descriptions>
          </div>
        </div>

        <div class="card">
          <div class="card__hd"><span class="card__title">链路口径</span></div>
          <div class="card__bd">
            <el-alert
              type="success"
              :closable="false"
              show-icon
              title="库存直接读 Redis 快照，是权威值"
              description="下单会写 order 库，库存流水由 worker 异步消费 Stream 落库，所以后台报表与这里的数字之间有几百毫秒的可见延迟——这是最终一致，不是 bug。"
            />
            <el-alert
              class="mt-sm"
              type="warning"
              :closable="false"
              show-icon
              title="忘了订单号？"
              description="再点一次抢购即可：后端幂等判定会把原订单号告诉你，不会重复扣库存。"
            />
            <el-alert
              class="mt-sm"
              type="info"
              :closable="false"
              show-icon
              title="没预热就抢不了"
              description="活动必须预热（写入 Redis 库存快照），否则会被判为 10004「不存在或不在活动时间内」。"
            />
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.mt {
  margin-top: 12px;
}

.mt-sm {
  margin-top: 12px;
}

.lot {
  font-size: 12px;
  color: var(--brand-600);
  margin-left: 6px;
}

.lot-body {
  display: flex;
  gap: 18px;
}

.lot-cover {
  width: 200px;
}

.lot-info {
  flex: 1;
  min-width: 0;
}

.lot-name {
  font-size: 19px;
  font-weight: 600;
  color: var(--ink-900);
  margin-bottom: 10px;
}

.lot-price {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 14px;
}

.lot-price .now {
  font-size: 28px;
  font-weight: 600;
  color: var(--brand-600);
}

.lot-price .was {
  font-size: 13px;
}

.dial {
  padding: 14px 0 10px;
  text-align: center;
}

.dial__caption {
  font-size: 12px;
  color: var(--ink-500);
  margin-bottom: 6px;
}

.dial__value {
  font-size: 44px;
  font-weight: 600;
  line-height: 1.15;
  color: var(--brand-600);
  letter-spacing: 0.02em;
}

.dial__value.is-dead {
  color: var(--ink-400);
}

.stock {
  margin: 16px 0;
}

.stock__head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 6px;
  font-size: 13px;
}

.stock__head b {
  font-size: 16px;
  color: var(--brand-600);
}

.cta {
  width: 100%;
  height: 46px;
  font-size: 16px;
  letter-spacing: 0.04em;
}

.result {
  margin-top: 18px;
  border-top: 1px dashed var(--line);
}

@media (max-width: 992px) {
  .lot-body {
    flex-direction: column;
  }

  .lot-cover {
    width: 100%;
  }
}
</style>
