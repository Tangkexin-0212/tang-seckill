<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { adminApi, catalogApi } from '@/api'
import type { Activity, Product } from '@/api/types'
import CoverPlate from '@/components/CoverPlate.vue'
import { useServerNow } from '@/composables/useServerClock'
import { feedback } from '@/utils/feedback'
import { dateTimeShort, discountLabel, duration, money } from '@/utils/format'
import { activityPhase, phaseMeta, stockColor, stockPercent } from '@/utils/status'

const router = useRouter()
const now = useServerNow()

const loading = ref(false)
const activities = ref<Activity[]>([])
const products = ref<Product[]>([])

/* ---------------- 列表 ---------------- */
async function load(): Promise<void> {
  loading.value = true
  try {
    const [acts, goods] = await Promise.all([catalogApi.activities(100), catalogApi.products(50, 0)])
    activities.value = acts ?? []
    products.value = goods ?? []
  } catch (err) {
    feedback.fromError(err)
  } finally {
    loading.value = false
  }
}

onMounted(load)

interface Row {
  activity: Activity
  phase: ReturnType<typeof activityPhase>
}

const rows = computed<Row[]>(() =>
  activities.value.map((a) => ({
    activity: a,
    phase: activityPhase(a.startTimeMs, a.endTimeMs, a.status, now.value),
  })),
)

/* ---------------- 搜索与分页（后端只有 limit，没有分页参数，故前端切片）---------------- */
const query = reactive({
  id: undefined as number | undefined,
  name: '',
  phase: '' as '' | 'upcoming' | 'live' | 'ended',
})
const page = ref(1)
const pageSize = ref(10)

const filtered = computed<Row[]>(() =>
  rows.value.filter((r) => {
    const a = r.activity
    if (query.id !== undefined && Number(query.id) > 0 && a.id !== Number(query.id)) return false
    if (query.name.trim() && !(a.productName || '').toLowerCase().includes(query.name.trim().toLowerCase()))
      return false
    if (query.phase && r.phase !== query.phase) return false
    return true
  }),
)

const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

watch([filtered, pageSize], () => {
  const maxPage = Math.max(1, Math.ceil(filtered.value.length / pageSize.value))
  if (page.value > maxPage) page.value = maxPage
})

function search(): void {
  page.value = 1
}

function resetQuery(): void {
  query.id = undefined
  query.name = ''
  query.phase = ''
  page.value = 1
}

/* ---------------- 行内操作 ---------------- */
const warmingId = ref<number | null>(null)
const warmingAll = ref(false)

async function warmup(a: Activity): Promise<void> {
  warmingId.value = a.id
  try {
    const res = await adminApi.warmup(a.id)
    if (res.warmed) feedback.success('预热完成', `LOT ${a.id} 的库存快照已写入 Redis`)
    else feedback.warning('预热被拒', '活动可能已结束')
  } catch (err) {
    feedback.fromError(err)
  } finally {
    warmingId.value = null
  }
}

/** 批量预热在槌场次：演示场景里最常用的操作 */
async function warmupLive(): Promise<void> {
  const targets = rows.value.filter((r) => r.phase === 'live').map((r) => r.activity)
  if (targets.length === 0) {
    feedback.warning('当前没有在槌的活动')
    return
  }
  const ok = await feedback.confirm(`将为 ${targets.length} 个在槌活动写入 Redis 库存快照，继续？`, '批量预热')
  if (!ok) return

  warmingAll.value = true
  let done = 0
  for (const a of targets) {
    try {
      const res = await adminApi.warmup(a.id)
      if (res.warmed) done += 1
    } catch {
      /* 单个失败不打断整批 */
    }
  }
  warmingAll.value = false
  feedback.success('批量预热完成', `${done} / ${targets.length} 成功`)
}

/* ---------------- 新增活动 ---------------- */
const dialogVisible = ref(false)
const submitting = ref(false)
const form = reactive({
  productId: undefined as number | undefined,
  priceYuan: 99,
  totalStock: 10,
  timeoutSeconds: 600,
  doWarmup: true,
})

function openCreate(): void {
  form.productId = undefined
  form.priceYuan = 99
  form.totalStock = 10
  form.timeoutSeconds = 600
  form.doWarmup = true
  dialogVisible.value = true
}

async function submitCreate(): Promise<void> {
  if (!form.productId) {
    feedback.warning('请选择商品')
    return
  }
  if (form.totalStock <= 0) {
    feedback.warning('总库存必须大于 0')
    return
  }

  submitting.value = true
  try {
    const created = await adminApi.createActivity({
      productId: form.productId,
      seckillPrice: Math.round(form.priceYuan * 100),
      totalStock: form.totalStock,
      // 只给 timeoutSeconds：活动起止时间交给后端按"现在"推导，
      // 避免前端本地时钟与服务端有偏差时把活动算到未来
      timeoutSeconds: form.timeoutSeconds,
    })

    if (form.doWarmup) {
      const warm = await adminApi.warmup(created.activityId)
      if (!warm.warmed) feedback.warning('活动已创建，但预热被拒')
    }

    feedback.success('活动已创建', `LOT ${created.activityId}`)
    dialogVisible.value = false
    await load()
    void router.push(`/seckill/grab/${created.activityId}`)
  } catch (err) {
    feedback.fromError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="app-page">
    <!-- ============ 查询条 ============ -->
    <div class="card">
      <div class="card__bd">
        <div class="search-bar">
          <el-form inline @submit.prevent>
            <el-form-item label="活动 ID">
              <el-input v-model.number="query.id" placeholder="精确匹配" clearable style="width: 140px" />
            </el-form-item>
            <el-form-item label="商品名称">
              <el-input v-model="query.name" placeholder="模糊匹配" clearable style="width: 200px" />
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="query.phase" placeholder="全部" clearable style="width: 140px">
                <el-option label="候场" value="upcoming" />
                <el-option label="在槌" value="live" />
                <el-option label="已落槌" value="ended" />
              </el-select>
            </el-form-item>
          </el-form>

          <div class="search-bar__actions">
            <el-button type="primary" @click="search">
              <el-icon><Search /></el-icon>查询
            </el-button>
            <el-button @click="resetQuery">
              <el-icon><Refresh /></el-icon>重置
            </el-button>
          </div>
        </div>
      </div>
    </div>

    <!-- ============ 列表 ============ -->
    <div class="card">
      <div class="toolbar">
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon>新增活动
        </el-button>
        <el-button :loading="warmingAll" @click="warmupLive">
          <el-icon><Download /></el-icon>批量预热在槌
        </el-button>
        <el-button :loading="loading" @click="load">
          <el-icon><Refresh /></el-icon>刷新
        </el-button>
        <div class="toolbar__fill" />
        <span class="text-soft">
          共 {{ filtered.length }} 条 · 在槌 {{ rows.filter((r) => r.phase === 'live').length }} 场
        </span>
      </div>

      <div class="card__bd card__bd--flush">
        <el-table v-loading="loading" :data="paged" style="width: 100%">
          <template #empty>
            <el-empty image-size="70" description="没有匹配的活动，点「新增活动」开一场" />
          </template>

          <el-table-column prop="activity.id" label="活动 ID" width="96" sortable>
            <template #default="{ row }: { row: Row }">
              <span class="mono text-brand">#{{ row.activity.id }}</span>
            </template>
          </el-table-column>

          <!-- 商品与价格合并成一列：8 列并排时状态列会被 fixed 操作列压住 -->
          <el-table-column label="商品 / 秒杀价" min-width="250">
            <template #default="{ row }: { row: Row }">
              <div class="goods">
                <CoverPlate :src="row.activity.cover" :name="row.activity.productName" size="sm" />
                <div class="goods__meta">
                  <span class="goods__name">{{ row.activity.productName || `商品 #${row.activity.productId}` }}</span>
                  <span class="goods__price">
                    <span class="mono price__now">{{ money(row.activity.seckillPrice) }}</span>
                    <span class="mono text-strike price__was">{{ money(row.activity.originalPrice) }}</span>
                    <el-tag
                      v-if="discountLabel(row.activity.seckillPrice, row.activity.originalPrice)"
                      size="small"
                      effect="plain"
                      type="primary"
                    >
                      {{ discountLabel(row.activity.seckillPrice, row.activity.originalPrice) }}
                    </el-tag>
                  </span>
                </div>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="库存" width="150" sortable prop="activity.stock">
            <template #default="{ row }: { row: Row }">
              <el-progress
                :percentage="stockPercent(row.activity.stock, row.activity.totalStock)"
                :stroke-width="8"
                :color="stockColor(row.activity.stock, row.activity.totalStock)"
                :format="() => `${row.activity.stock}/${row.activity.totalStock}`"
              />
            </template>
          </el-table-column>

          <el-table-column label="活动时间" width="168">
            <template #default="{ row }: { row: Row }">
              <span class="mono time">
                {{ dateTimeShort(row.activity.startTimeMs) }}
                <span class="time__to">→ {{ dateTimeShort(row.activity.endTimeMs) }}</span>
              </span>
            </template>
          </el-table-column>

          <el-table-column label="状态" width="136">
            <template #default="{ row }: { row: Row }">
              <el-tag :type="phaseMeta(row.phase).type" size="small" effect="light">
                {{ phaseMeta(row.phase).label }}
              </el-tag>
              <div v-if="row.phase === 'live'" class="mono countdown">
                距落槌 {{ duration(row.activity.endTimeMs - now) }}
              </div>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="182" fixed="right">
            <template #default="{ row }: { row: Row }">
              <div class="op-links">
                <el-button link type="primary" @click="router.push(`/seckill/grab/${row.activity.id}`)">
                  <el-icon><Pointer /></el-icon>抢购台
                </el-button>
                <el-button
                  link
                  type="success"
                  :loading="warmingId === row.activity.id"
                  @click="warmup(row.activity)"
                >
                  预热
                </el-button>
                <el-button link type="primary" @click="feedback.copy(String(row.activity.id), '活动 ID')">
                  复制 ID
                </el-button>
              </div>
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

    <!-- ============ 新增活动 ============ -->
    <el-dialog v-model="dialogVisible" title="新增秒杀活动" width="520px" append-to-body>
      <el-form label-width="96px" @submit.prevent>
        <el-form-item label="商品" required>
          <el-select v-model="form.productId" filterable placeholder="选择商品" style="width: 100%">
            <el-option
              v-for="p in products"
              :key="p.id"
              :label="`${p.name}（${money(p.price)}）`"
              :value="p.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="秒杀价（元）" required>
          <el-input-number v-model="form.priceYuan" :min="0.01" :precision="2" :step="10" style="width: 180px" />
        </el-form-item>
        <el-form-item label="总库存" required>
          <el-input-number v-model="form.totalStock" :min="1" :step="1" style="width: 180px" />
        </el-form-item>
        <el-form-item label="活动时长">
          <el-input-number v-model="form.timeoutSeconds" :min="30" :step="60" style="width: 180px" />
          <span class="text-soft form-hint">秒，从提交时刻开始计时</span>
        </el-form-item>
        <el-form-item label="立即预热">
          <el-switch v-model="form.doWarmup" />
          <span class="text-soft form-hint">不预热的活动会被判为 10004（不存在或不在活动时间内）</span>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitCreate">确定创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.goods {
  display: flex;
  align-items: center;
  gap: 8px;
}

.goods__meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 2px;
}

.goods__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ink-900);
}

.goods__price {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.price__now {
  color: var(--brand-600);
  font-weight: 600;
}

.price__was {
  font-size: 12px;
}

.time {
  font-size: 12px;
  color: var(--ink-700);
}

.time__to {
  color: var(--ink-400);
}

.countdown {
  margin-top: 3px;
  font-size: 12px;
  color: var(--green-600);
}

.pager {
  padding: 0 16px 14px;
}

.form-hint {
  margin-left: 10px;
  font-size: 12px;
}
</style>
