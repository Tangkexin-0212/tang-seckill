<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { catalogApi, orderApi, systemApi } from '@/api'
import type { Activity, Order } from '@/api/types'
import CoverPlate from '@/components/CoverPlate.vue'
import { useServerNow } from '@/composables/useServerClock'
import { useAuth } from '@/stores/auth'
import { useCart } from '@/stores/cart'
import { feedback } from '@/utils/feedback'
import { dateTime, duration, money } from '@/utils/format'
import { activityPhase, orderStatusMeta, stockColor, stockPercent } from '@/utils/status'

const router = useRouter()
const auth = useAuth()
const cart = useCart()
const now = useServerNow()

const activities = ref<Activity[]>([])
const orders = ref<Order[]>([])
const health = ref<{ code: number; msg: string } | null>(null)
const loading = ref(true)

async function load(): Promise<void> {
  loading.value = true
  const [acts, h, ords] = await Promise.allSettled([
    catalogApi.activities(50),
    systemApi.health(),
    auth.isLoggedIn ? orderApi.list(20) : Promise.resolve<Order[]>([]),
  ])

  if (acts.status === 'fulfilled') activities.value = acts.value ?? []
  else feedback.fromError(acts.reason)

  if (h.status === 'fulfilled') health.value = h.value
  if (ords.status === 'fulfilled') orders.value = ords.value ?? []

  loading.value = false
}

onMounted(load)

/* ---------------- 概览指标 ---------------- */
const liveActs = computed(() =>
  activities.value.filter(
    (a) => activityPhase(a.startTimeMs, a.endTimeMs, a.status, now.value) === 'live',
  ),
)

const remainTotal = computed(() => activities.value.reduce((s, a) => s + Math.max(0, a.stock), 0))
const stockTotal = computed(() => activities.value.reduce((s, a) => s + Math.max(0, a.totalStock), 0))
const gonePercent = computed(() =>
  stockTotal.value > 0 ? Math.round(((stockTotal.value - remainTotal.value) / stockTotal.value) * 100) : 0,
)

const pendingOrders = computed(() => orders.value.filter((o) => o.status === 0))
const paidAmount = computed(() =>
  orders.value.filter((o) => o.status === 1).reduce((s, o) => s + o.amount, 0),
)

/** 去化最高的 5 场：仪表盘上最该被盯住的数字 */
const hotLots = computed(() =>
  [...activities.value]
    .filter((a) => a.totalStock > 0)
    .map((a) => ({ ...a, gone: 100 - stockPercent(a.stock, a.totalStock) }))
    .sort((a, b) => b.gone - a.gone)
    .slice(0, 5),
)

const recentOrders = computed(() =>
  [...orders.value].sort((a, b) => b.createdAtMs - a.createdAtMs).slice(0, 6),
)

function remainText(o: Order): string {
  const left = o.expireAtMs - now.value
  return left > 0 ? duration(left) : '已超时'
}

function actPhase(a: Activity) {
  return activityPhase(a.startTimeMs, a.endTimeMs, a.status, now.value)
}
</script>

<template>
  <div class="app-page">
    <!-- ============ 概览 ============ -->
    <el-row :gutter="12">
      <el-col :xs="24" :sm="12" :md="6">
        <div class="stat">
          <span class="stat__icon"><el-icon><Timer /></el-icon></span>
          <div>
            <div class="stat__label">在槌场次</div>
            <div class="stat__value">{{ liveActs.length }}<small>/ {{ activities.length }} 场</small></div>
          </div>
        </div>
      </el-col>
      <el-col :xs="24" :sm="12" :md="6">
        <div class="stat stat--green">
          <span class="stat__icon"><el-icon><Box /></el-icon></span>
          <div>
            <div class="stat__label">剩余库存</div>
            <div class="stat__value">{{ remainTotal }}<small>/ {{ stockTotal }} 件</small></div>
          </div>
        </div>
      </el-col>
      <el-col :xs="24" :sm="12" :md="6">
        <div class="stat stat--amber">
          <span class="stat__icon"><el-icon><TrendCharts /></el-icon></span>
          <div>
            <div class="stat__label">整体去化</div>
            <div class="stat__value">{{ gonePercent }}<small>%</small></div>
          </div>
        </div>
      </el-col>
      <el-col :xs="24" :sm="12" :md="6">
        <div class="stat stat--rose">
          <span class="stat__icon"><el-icon><Wallet /></el-icon></span>
          <div>
            <div class="stat__label">我的待支付</div>
            <div class="stat__value">
              {{ pendingOrders.length }}<small>{{ paidAmount > 0 ? `已付 ${money(paidAmount)}` : '单' }}</small>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="12" class="mt">
      <!-- ============ 最近订单 ============ -->
      <el-col :xs="24" :lg="16">
        <div class="card">
          <div class="card__hd">
            <span class="card__title">最近订单</span>
            <div class="card__extra">
              <el-button link type="primary" @click="router.push('/order/list')">查看全部</el-button>
              <el-button link type="primary" @click="load"><el-icon><Refresh /></el-icon>刷新</el-button>
            </div>
          </div>

          <div class="card__bd card__bd--flush">
            <el-table v-loading="loading" :data="recentOrders" style="width: 100%">
              <template #empty>
                <el-empty
                  :image-size="70"
                  :description="auth.isLoggedIn ? '还没有订单，去活动列表抢一件试试' : '登录后可查看订单'"
                />
              </template>
              <el-table-column type="index" label="#" width="52" />
              <el-table-column prop="orderNo" label="订单号" min-width="190">
                <template #default="{ row }: { row: Order }">
                  <span class="mono text-brand">{{ row.orderNo }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="productName" label="商品" min-width="150" show-overflow-tooltip />
              <el-table-column prop="amount" label="金额" width="110" align="right">
                <template #default="{ row }: { row: Order }">
                  <span class="mono">{{ money(row.amount) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="110">
                <template #default="{ row }: { row: Order }">
                  <el-tag :type="orderStatusMeta(row.status).type" size="small" effect="light">
                    {{ orderStatusMeta(row.status).label }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="剩余支付时间" width="130">
                <template #default="{ row }: { row: Order }">
                  <span v-if="row.status === 0" class="mono text-danger">{{ remainText(row) }}</span>
                  <span v-else class="mono text-soft">{{ dateTime(row.createdAtMs) }}</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-col>

      <!-- ============ 右侧 ============ -->
      <el-col :xs="24" :lg="8">
        <div class="card">
          <div class="card__hd"><span class="card__title">系统状态</span></div>
          <div class="card__bd">
            <el-descriptions :column="1" border size="small">
              <el-descriptions-item label="网关">
                <el-tag v-if="health && health.code === 0" type="success" size="small">正常</el-tag>
                <el-tag v-else type="danger" size="small">不可达</el-tag>
                <span class="mono text-soft ml">127.0.0.1:8080</span>
              </el-descriptions-item>
              <el-descriptions-item label="活动总数">{{ activities.length }} 场</el-descriptions-item>
              <el-descriptions-item label="购物车">
                {{ cart.badgeCount }} 种<template v-if="cart.count > 0"> / {{ cart.count }} 件</template>
              </el-descriptions-item>
              <el-descriptions-item label="登录态">
                <span v-if="auth.isLoggedIn" class="mono">UID {{ auth.uid }} · {{ duration(auth.msLeft) }}</span>
                <span v-else class="text-soft">未登录</span>
              </el-descriptions-item>
              <el-descriptions-item label="库存口径">Redis 权威 / MySQL 最终一致</el-descriptions-item>
            </el-descriptions>

            <div class="quick">
              <el-button type="primary" plain @click="router.push('/seckill/activity')">
                <el-icon><Pointer /></el-icon>去抢购
              </el-button>
              <el-button type="success" plain @click="router.push('/system/tool')">
                <el-icon><Tools /></el-icon>开一场活动
              </el-button>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card__hd">
            <span class="card__title">去化最快的场次</span>
            <div class="card__extra">
              <el-tag size="small" type="info" effect="plain">Top 5</el-tag>
            </div>
          </div>
          <div class="card__bd">
            <p v-if="hotLots.length === 0" class="text-soft">暂无活动数据</p>
            <div v-for="a in hotLots" :key="a.id" class="hot">
              <div class="hot__head">
                <span class="mono text-brand">LOT {{ String(a.id).padStart(3, '0') }}</span>
                <span class="hot__name">{{ a.productName }}</span>
                <span class="mono hot__num">{{ a.gone }}%</span>
              </div>
              <el-progress
                :percentage="a.gone"
                :stroke-width="6"
                :color="stockColor(a.stock, a.totalStock)"
                :show-text="false"
              />
            </div>
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- ============ 在槌速览 ============ -->
    <div class="card mt">
      <div class="card__hd">
        <span class="card__title">在槌场次速览</span>
        <div class="card__extra">
          <el-button link type="primary" @click="router.push('/seckill/activity')">
            进入活动列表<el-icon><Right /></el-icon>
          </el-button>
        </div>
      </div>
      <div class="card__bd">
        <p v-if="liveActs.length === 0" class="text-soft">当前没有在槌的活动，去「系统工具」开一场。</p>
        <el-row v-else :gutter="12">
          <el-col v-for="a in liveActs.slice(0, 4)" :key="a.id" :xs="24" :sm="12" :lg="6">
            <div class="mini" @click="router.push(`/seckill/grab/${a.id}`)">
              <div class="mini__top">
                <CoverPlate :src="a.cover" :name="a.productName" size="sm" />
                <div class="mini__meta">
                  <span class="mini__name">{{ a.productName }}</span>
                  <span class="mono mini__price">{{ money(a.seckillPrice) }}</span>
                </div>
                <el-tag size="small" :type="actPhase(a) === 'live' ? 'success' : 'info'">
                  {{ actPhase(a) === 'live' ? '在槌' : actPhase(a) === 'upcoming' ? '候场' : '已落槌' }}
                </el-tag>
              </div>
              <el-progress
                :percentage="stockPercent(a.stock, a.totalStock)"
                :stroke-width="6"
                :color="stockColor(a.stock, a.totalStock)"
                :show-text="false"
              />
              <div class="mini__foot">
                <span class="text-soft">剩余 {{ a.stock }} / {{ a.totalStock }}</span>
                <span class="mono">{{ duration(a.endTimeMs - now) }}</span>
              </div>
            </div>
          </el-col>
        </el-row>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mt {
  margin-top: 12px;
}

.ml {
  margin-left: 8px;
}

.quick {
  display: flex;
  gap: 8px;
  margin-top: 14px;
}

.quick .el-button {
  flex: 1;
  margin-left: 0;
}

.hot + .hot {
  margin-top: 14px;
}

.hot__head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 5px;
  font-size: 13px;
}

.hot__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ink-700);
}

.hot__num {
  color: var(--ink-500);
}

.mini {
  padding: 12px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--bg-soft);
  cursor: pointer;
  transition:
    border-color var(--t-fast) var(--ease),
    box-shadow var(--t-fast) var(--ease);
}

.mini:hover {
  border-color: var(--brand-300);
  box-shadow: 0 2px 10px rgba(22, 119, 255, 0.12);
}

.mini__top {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}

.mini__meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.mini__name {
  font-size: 13px;
  color: var(--ink-900);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini__price {
  font-size: 13px;
  color: var(--brand-600);
  font-weight: 600;
}

.mini__foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
  font-size: 12px;
}
</style>
