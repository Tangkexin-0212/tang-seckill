<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { orderApi, seckillApi } from '@/api'
import type { Order } from '@/api/types'
import { useServerNow } from '@/composables/useServerClock'
import { feedback } from '@/utils/feedback'
import { dateTime, duration, money, shortOrderNo } from '@/utils/format'
import { orderStatusMeta } from '@/utils/status'

const router = useRouter()
const now = useServerNow()

const loading = ref(false)
const orders = ref<Order[]>([])
const busy = ref('')

const query = reactive({ orderNo: '', status: '' as '' | '0' | '1' | '2' })
const page = ref(1)
const pageSize = ref(10)

async function load(): Promise<void> {
  loading.value = true
  try {
    orders.value = (await orderApi.list(100)) ?? []
  } catch (err) {
    feedback.fromError(err)
  } finally {
    loading.value = false
  }
}

onMounted(load)

const filtered = computed(() =>
  orders.value
    .filter((o) => {
      if (query.orderNo.trim() && !o.orderNo.toLowerCase().includes(query.orderNo.trim().toLowerCase()))
        return false
      if (query.status !== '' && String(o.status) !== query.status) return false
      return true
    })
    .sort((a, b) => {
      // 待支付排最前且最紧急的在上，其余按下单时间倒序
      if (a.status === 0 && b.status !== 0) return -1
      if (a.status !== 0 && b.status === 0) return 1
      if (a.status === 0 && b.status === 0) return a.expireAtMs - b.expireAtMs
      return b.createdAtMs - a.createdAtMs
    }),
)

const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

const pendingCount = computed(() => orders.value.filter((o) => o.status === 0).length)
const paidAmount = computed(() =>
  orders.value.filter((o) => o.status === 1).reduce((s, o) => s + o.amount, 0),
)

watch([filtered, pageSize], () => {
  const maxPage = Math.max(1, Math.ceil(filtered.value.length / pageSize.value))
  if (page.value > maxPage) page.value = maxPage
})

function remainMs(o: Order): number {
  return o.expireAtMs - now.value
}

function remainText(o: Order): string {
  const left = remainMs(o)
  return left > 0 ? duration(left) : '已超时'
}

function urgent(o: Order): boolean {
  const left = remainMs(o)
  return left > 0 && left < 120_000
}

/* ---------------- 支付 / 放弃 ---------------- */
async function pay(o: Order): Promise<void> {
  busy.value = o.orderNo
  try {
    const res = await orderApi.pay(o.orderNo)
    feedback.success('支付成功', `${res.productName || '拍品'} ${money(res.amount)}`)
  } catch (err) {
    feedback.fromError(err)
  } finally {
    busy.value = ''
    await load()
  }
}

async function cancel(o: Order): Promise<void> {
  const ok = await feedback.confirm(
    `放弃订单 ${shortOrderNo(o.orderNo)} 后库存会立刻回补给其他人，且该活动可以重新参抢。确定继续？`,
    '放弃订单',
    '确定放弃',
  )
  if (!ok) return

  busy.value = o.orderNo
  try {
    const res = await seckillApi.cancel(o.activityId)
    if (res.released) feedback.success('已放弃', '库存已回补')
    else feedback.warning('没有可释放的资格', '订单状态可能已变化')
  } catch (err) {
    feedback.fromError(err)
  } finally {
    busy.value = ''
    await load()
  }
}

function resetQuery(): void {
  query.orderNo = ''
  query.status = ''
  page.value = 1
}
</script>

<template>
  <div class="app-page">
    <div class="card">
      <div class="card__bd">
        <div class="search-bar">
          <el-form inline @submit.prevent>
            <el-form-item label="订单号">
              <el-input v-model="query.orderNo" placeholder="支持模糊匹配" clearable style="width: 240px" />
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="query.status" placeholder="全部" clearable style="width: 130px">
                <el-option label="待支付" value="0" />
                <el-option label="已支付" value="1" />
                <el-option label="已取消" value="2" />
              </el-select>
            </el-form-item>
          </el-form>
          <div class="search-bar__actions">
            <el-button type="primary" @click="page = 1"><el-icon><Search /></el-icon>查询</el-button>
            <el-button @click="resetQuery"><el-icon><Refresh /></el-icon>重置</el-button>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="toolbar">
        <el-button :loading="loading" @click="load"><el-icon><Refresh /></el-icon>刷新</el-button>
        <el-button type="primary" plain @click="router.push('/seckill/activity')">
          <el-icon><Pointer /></el-icon>去抢购
        </el-button>
        <div class="toolbar__fill" />
        <el-tag v-if="pendingCount > 0" type="warning" effect="light">待支付 {{ pendingCount }} 单</el-tag>
        <span class="text-soft">累计成交 {{ money(paidAmount) }}</span>
      </div>

      <div class="card__bd card__bd--flush">
        <el-table v-loading="loading" :data="paged" style="width: 100%" :row-class-name="() => ''">
          <template #empty>
            <el-empty image-size="70" description="还没有订单，去活动列表抢一件" />
          </template>

          <el-table-column label="订单号" min-width="220">
            <template #default="{ row }: { row: Order }">
              <div class="orderno">
                <span class="mono ordemo__text">{{ row.orderNo }}</span>
                <el-button link type="primary" @click="feedback.copy(row.orderNo, '订单号')">复制</el-button>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="商品" min-width="180">
            <template #default="{ row }: { row: Order }">
              <div class="lot mono">LOT {{ String(row.activityId).padStart(3, '0') }}</div>
              <div class="pname">{{ row.productName || `商品 #${row.productId}` }}</div>
            </template>
          </el-table-column>

          <el-table-column label="金额" width="120" align="right" sortable prop="amount">
            <template #default="{ row }: { row: Order }">
              <span class="mono amount">{{ money(row.amount) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="状态" width="110">
            <template #default="{ row }: { row: Order }">
              <el-tag :type="orderStatusMeta(row.status).type" size="small" effect="light">
                {{ orderStatusMeta(row.status).label }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="下单时间" width="170">
            <template #default="{ row }: { row: Order }">
              <span class="mono text-soft">{{ dateTime(row.createdAtMs) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="剩余支付时间" width="160">
            <template #default="{ row }: { row: Order }">
              <template v-if="row.status === 0">
                <span class="mono remain" :class="{ 'is-urgent': urgent(row) }">{{ remainText(row) }}</span>
                <el-progress
                  :percentage="Math.max(0, Math.min(100, Math.round((remainMs(row) / (row.expireAtMs - row.createdAtMs)) * 100)))"
                  :stroke-width="4"
                  :show-text="false"
                  :color="urgent(row) ? '#e34d59' : '#18a058'"
                />
              </template>
              <span v-else class="mono text-soft">{{ row.status === 1 ? dateTime(row.paidAtMs) : '—' }}</span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="150" fixed="right">
            <template #default="{ row }: { row: Order }">
              <div v-if="row.status === 0" class="op-links">
                <el-button link type="success" :loading="busy === row.orderNo" @click="pay(row)">
                  <el-icon><Money /></el-icon>支付
                </el-button>
                <el-button link type="danger" :loading="busy === row.orderNo" @click="cancel(row)">
                  放弃
                </el-button>
              </div>
              <el-button v-else link type="primary" @click="router.push(`/seckill/grab/${row.activityId}`)">
                查看活动
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="pager">
          <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50]"
            :total="filtered.length"
            layout="total, sizes, prev, pager, next, jumper"
            background
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.orderno {
  display: flex;
  align-items: center;
  gap: 8px;
}

.ordemo__text {
  color: var(--brand-600);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lot {
  font-size: 12px;
  color: var(--brand-600);
}

.pname {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ink-900);
}

.amount {
  font-weight: 600;
  color: var(--ink-900);
}

.remain {
  display: block;
  margin-bottom: 4px;
  font-size: 12px;
  color: var(--green-600);
}

.remain.is-urgent {
  color: var(--danger);
}

.pager {
  padding: 0 16px 14px;
}
</style>
