<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { adminApi, catalogApi, systemApi } from '@/api'
import { ApiError } from '@/api/client'
import { describeError } from '@/api/errors'
import { useAuth } from '@/stores/auth'
import { feedback } from '@/utils/feedback'
import { money } from '@/utils/format'

const auth = useAuth()
const router = useRouter()

/* ---------------- 控制台 ---------------- */
type LogKind = 'info' | 'ok' | 'err'

interface LogLine {
  id: number
  at: string
  kind: LogKind
  text: string
}

const lines = ref<LogLine[]>([])
let seq = 0

function say(kind: LogKind, text: string): void {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  lines.value.unshift({ id: ++seq, at: `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`, kind, text })
  if (lines.value.length > 80) lines.value.pop()
}

function logError(scope: string, err: unknown): void {
  const e = err instanceof ApiError ? err : null
  const p = describeError(err)
  say('err', `${scope} 失败 · ${p.title}${e ? `（code ${e.code}）` : ''}`)
  feedback.fromError(err)
}

function toCents(yuan: number): number {
  return Math.round((Number(yuan) || 0) * 100)
}

/* ---------------- 链路自检 ---------------- */
const checking = ref(false)
const checkResult = ref<{ gateway: string; activities: string; products: string }>({
  gateway: '未检测',
  activities: '未检测',
  products: '未检测',
})

async function runCheck(): Promise<void> {
  checking.value = true
  say('info', '开始链路自检')
  checkResult.value = { gateway: '检测中', activities: '检测中', products: '检测中' }

  try {
    const h = await systemApi.health()
    checkResult.value.gateway = `code=${h.code} ${h.msg}`
    say('ok', `gateway /health → code=${h.code} msg=${h.msg}`)
  } catch (err) {
    checkResult.value.gateway = '不可达'
    logError('gateway /health', err)
  }

  try {
    const acts = await catalogApi.activities(50)
    const live = acts.filter((a) => a.status === 1).length
    checkResult.value.activities = `${acts.length} 场（在槌 ${live}）`
    say('ok', `product 服务可读：活动 ${acts.length} 场（在槌 ${live}）`)
  } catch (err) {
    checkResult.value.activities = '失败'
    logError('activity/list', err)
  }

  try {
    const goods = await catalogApi.products(1, 0)
    checkResult.value.products = `抽样 ${goods.length} 条`
    say('ok', `商品库可读：抽样 ${goods.length} 条`)
  } catch (err) {
    checkResult.value.products = '失败'
    logError('product/list', err)
  }

  say('info', auth.isLoggedIn ? `当前登录 UID ${auth.uid}` : '当前未登录（购物车/订单链路未覆盖）')
  checking.value = false
}

/* ---------------- 一键开一场 ---------------- */
const quick = reactive({
  name: `秒杀拍品 ${new Date().toLocaleTimeString('zh-CN')}`,
  priceYuan: 1999,
  seckillPriceYuan: 99,
  totalStock: 10,
  timeoutSeconds: 600,
})
const running = ref(false)
const lastActivityId = ref(0)

async function oneShot(): Promise<void> {
  if (!quick.name.trim()) {
    feedback.warning('商品名不能为空')
    return
  }
  running.value = true
  lastActivityId.value = 0
  say('info', `开始一键开市：${quick.name}`)

  try {
    const product = await adminApi.createProduct({
      name: quick.name.trim(),
      price: toCents(quick.priceYuan),
      description: '系统工具一键创建',
    })
    say('ok', `商品已创建 · productId=${product.productId}`)

    const activity = await adminApi.createActivity({
      productId: product.productId,
      seckillPrice: toCents(quick.seckillPriceYuan),
      totalStock: Number(quick.totalStock),
      timeoutSeconds: Number(quick.timeoutSeconds),
    })
    say('ok', `活动已创建 · activityId=${activity.activityId}`)

    const warm = await adminApi.warmup(activity.activityId)
    if (!warm.warmed) {
      say('err', '预热被拒：活动可能已结束')
      feedback.warning('活动已创建但预热失败')
      return
    }
    say('ok', '库存快照已写入 Redis，活动可抢')
    lastActivityId.value = activity.activityId
    feedback.success('一场新的秒杀已就绪', `LOT ${activity.activityId}`)
  } catch (err) {
    logError('一键开市', err)
  } finally {
    running.value = false
  }
}

/* ---------------- 分步操作 ---------------- */
const productForm = reactive({ name: '', priceYuan: 1999, cover: '', description: '' })
const activityForm = reactive({ productId: undefined as number | undefined, seckillPriceYuan: 99, totalStock: 10, timeoutSeconds: 600 })
const warmupId = ref<number | undefined>(undefined)
const busy = ref(false)

async function createProduct(): Promise<void> {
  if (!productForm.name.trim()) {
    feedback.warning('商品名不能为空')
    return
  }
  busy.value = true
  say('info', `POST /api/admin/product name=${productForm.name}`)
  try {
    const res = await adminApi.createProduct({
      name: productForm.name.trim(),
      price: toCents(productForm.priceYuan),
      cover: productForm.cover.trim(),
      description: productForm.description.trim(),
    })
    say('ok', `商品已创建 · productId=${res.productId}（${money(toCents(productForm.priceYuan))}）`)
    feedback.success('商品已创建', `ID ${res.productId}`)
    activityForm.productId = res.productId
  } catch (err) {
    logError('创建商品', err)
  } finally {
    busy.value = false
  }
}

async function createActivity(): Promise<void> {
  const pid = Number(activityForm.productId)
  if (!pid || pid <= 0) {
    feedback.warning('需要先有一个商品 ID')
    return
  }
  busy.value = true
  say('info', `POST /api/admin/activity productId=${pid} stock=${activityForm.totalStock}`)
  try {
    const res = await adminApi.createActivity({
      productId: pid,
      seckillPrice: toCents(activityForm.seckillPriceYuan),
      totalStock: Number(activityForm.totalStock),
      timeoutSeconds: Number(activityForm.timeoutSeconds),
    })
    say('ok', `活动已创建 · activityId=${res.activityId}`)
    feedback.success('活动已创建', `LOT ${res.activityId}`)
    warmupId.value = res.activityId
  } catch (err) {
    logError('创建活动', err)
  } finally {
    busy.value = false
  }
}

async function warmup(): Promise<void> {
  const id = Number(warmupId.value)
  if (!id || id <= 0) {
    feedback.warning('需要活动 ID')
    return
  }
  busy.value = true
  say('info', `POST /api/admin/warmup activityId=${id}`)
  try {
    const res = await adminApi.warmup(id)
    if (res.warmed) {
      say('ok', '预热完成 · 库存已写入 Redis')
      feedback.success('预热完成', `LOT ${id}`)
    } else {
      say('err', '预热被拒（活动可能已结束）')
      feedback.warning('预热被拒')
    }
  } catch (err) {
    logError('预热', err)
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  say('info', '系统工具就绪。管理接口（/api/admin/*）当前无鉴权，仅供本地实验。')
})

const SERVICES = [
  { name: 'gateway', port: '8080', desc: 'Hertz 网关 · 鉴权与路由' },
  { name: 'user', port: '9001', desc: '账号与 Token' },
  { name: 'product', port: '9002', desc: '商品与活动' },
  { name: 'stock', port: '9003', desc: '库存账本' },
  { name: 'cart', port: '9004', desc: 'Redis 购物车' },
  { name: 'seckill', port: '9005', desc: '秒杀编排（限流/扣减/建单）' },
  { name: 'order', port: '9006', desc: '订单与延时队列' },
  { name: 'cron', port: '—', desc: '活动同步与库存对账' },
  { name: 'worker', port: '—', desc: 'Stream 消费落库 + 超时释放' },
]

const busyAll = computed(() => busy.value || running.value)
</script>

<template>
  <div class="app-page">
    <el-row :gutter="12">
      <!-- ==================== 左列 ==================== -->
      <el-col :xs="24" :lg="14">
        <div class="card">
          <div class="card__hd">
            <span class="card__title">一键开一场</span>
            <div class="card__extra">
              <el-tag size="small" type="info" effect="plain">建商品 → 建活动 → 预热</el-tag>
            </div>
          </div>
          <div class="card__bd">
            <el-form label-width="106px" @submit.prevent>
              <el-form-item label="商品名">
                <el-input v-model="quick.name" :disabled="running" />
              </el-form-item>
              <el-row :gutter="12">
                <el-col :span="12">
                  <el-form-item label="原价（元）">
                    <el-input-number v-model="quick.priceYuan" :min="0.01" :precision="2" :step="100" style="width: 100%" :disabled="running" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="秒杀价（元）">
                    <el-input-number v-model="quick.seckillPriceYuan" :min="0.01" :precision="2" :step="10" style="width: 100%" :disabled="running" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="总库存">
                    <el-input-number v-model="quick.totalStock" :min="1" style="width: 100%" :disabled="running" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="活动时长（秒）">
                    <el-input-number v-model="quick.timeoutSeconds" :min="30" :step="60" style="width: 100%" :disabled="running" />
                  </el-form-item>
                </el-col>
              </el-row>
            </el-form>

            <div class="actions">
              <el-button type="primary" :loading="running" @click="oneShot">
                <el-icon><Pointer /></el-icon>开一场
              </el-button>
              <el-button v-if="lastActivityId" type="success" plain @click="router.push(`/seckill/grab/${lastActivityId}`)">
                前往 LOT {{ String(lastActivityId).padStart(3, '0') }}
              </el-button>
            </div>

            <el-alert
              class="mt-sm"
              type="info"
              :closable="false"
              show-icon
              title="活动从提交时刻开始计时"
              description="起止时间交给后端按 timeoutSeconds 推导，前端不传时间戳——避免本地时钟与服务端有偏差时把活动算到未来。"
            />
          </div>
        </div>

        <div class="card">
          <div class="card__hd"><span class="card__title">分步操作</span></div>
          <div class="card__bd">
            <el-form label-width="106px" @submit.prevent>
              <el-form-item label="商品名">
                <el-input v-model="productForm.name" placeholder="例如：夜场机械键盘" :disabled="busyAll" />
              </el-form-item>
              <el-row :gutter="12">
                <el-col :span="12">
                  <el-form-item label="价格（元）">
                    <el-input-number v-model="productForm.priceYuan" :min="0.01" :precision="2" :step="100" style="width: 100%" :disabled="busyAll" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="封面 URL">
                    <el-input v-model="productForm.cover" placeholder="可空" :disabled="busyAll" />
                  </el-form-item>
                </el-col>
              </el-row>
              <el-form-item label="描述">
                <el-input v-model="productForm.description" placeholder="可选" :disabled="busyAll" />
              </el-form-item>
            </el-form>
            <el-button :disabled="busyAll" @click="createProduct">
              <el-icon><Plus /></el-icon>创建商品
            </el-button>

            <el-divider />

            <el-form label-width="106px" @submit.prevent>
              <el-row :gutter="12">
                <el-col :span="12">
                  <el-form-item label="商品 ID">
                    <el-input-number v-model="activityForm.productId" :min="1" style="width: 100%" :disabled="busyAll" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="秒杀价（元）">
                    <el-input-number v-model="activityForm.seckillPriceYuan" :min="0.01" :precision="2" style="width: 100%" :disabled="busyAll" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="总库存">
                    <el-input-number v-model="activityForm.totalStock" :min="1" style="width: 100%" :disabled="busyAll" />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="时长（秒）">
                    <el-input-number v-model="activityForm.timeoutSeconds" :min="30" :step="60" style="width: 100%" :disabled="busyAll" />
                  </el-form-item>
                </el-col>
              </el-row>
            </el-form>
            <el-button :disabled="busyAll" @click="createActivity">
              <el-icon><Plus /></el-icon>创建活动
            </el-button>

            <el-divider />

            <el-form label-width="106px" @submit.prevent>
              <el-form-item label="活动 ID">
                <el-input-number v-model="warmupId" :min="1" style="width: 220px" :disabled="busyAll" />
              </el-form-item>
            </el-form>
            <el-button type="success" :disabled="busyAll" @click="warmup">
              <el-icon><Download /></el-icon>写入 Redis 库存快照
            </el-button>
            <span class="hint">没预热的活动会被判为 10004</span>
          </div>
        </div>
      </el-col>

      <!-- ==================== 右列 ==================== -->
      <el-col :xs="24" :lg="10">
        <div class="card">
          <div class="card__hd">
            <span class="card__title">链路自检</span>
            <div class="card__extra">
              <el-button :loading="checking" type="primary" plain @click="runCheck">
                <el-icon><Connection /></el-icon>开始自检
              </el-button>
            </div>
          </div>
          <div class="card__bd">
            <el-descriptions :column="1" border size="small">
              <el-descriptions-item label="gateway /health">
                <span class="mono">{{ checkResult.gateway }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="activity/list">
                <span class="mono">{{ checkResult.activities }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="product/list">
                <span class="mono">{{ checkResult.products }}</span>
              </el-descriptions-item>
            </el-descriptions>
          </div>
        </div>

        <div class="card">
          <div class="card__hd"><span class="card__title">服务清单</span></div>
          <div class="card__bd svc">
            <div v-for="s in SERVICES" :key="s.name" class="svc__row">
              <span class="mono svc__name">{{ s.name }}</span>
              <span class="mono svc__port">{{ s.port }}</span>
              <span class="svc__desc">{{ s.desc }}</span>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="card__hd">
            <span class="card__title">控制台</span>
            <div class="card__extra">
              <el-button link type="primary" :disabled="lines.length === 0" @click="lines = []">清屏</el-button>
            </div>
          </div>
          <div class="card__bd">
            <div class="console">
              <p v-if="lines.length === 0" class="text-soft">等待操作…</p>
              <div v-for="l in lines" :key="l.id" class="console__line" :class="`console__line--${l.kind}`">
                <span class="console__at">{{ l.at }}</span>
                <span class="console__text">{{ l.text }}</span>
              </div>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.mt-sm {
  margin-top: 12px;
}

.actions {
  display: flex;
  gap: 8px;
}

.hint {
  margin-left: 10px;
  font-size: 12px;
  color: var(--ink-400);
}

.svc__row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 0;
  font-size: 12px;
  border-bottom: 1px dashed var(--line-2);
}

.svc__row:last-child {
  border-bottom: 0;
}

.svc__name {
  width: 62px;
  color: var(--brand-600);
}

.svc__port {
  width: 40px;
  color: var(--ink-500);
}

.svc__desc {
  flex: 1;
  min-width: 0;
  color: var(--ink-600);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
