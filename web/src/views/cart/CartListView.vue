<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import CoverPlate from '@/components/CoverPlate.vue'
import { useCart } from '@/stores/cart'
import { feedback } from '@/utils/feedback'
import { money } from '@/utils/format'

const cart = useCart()
const router = useRouter()

onMounted(() => {
  cart.load().catch((err) => feedback.fromError(err))
})

const rows = computed(() => cart.items)

async function changeQty(productId: number, value: number | undefined): Promise<void> {
  const qty = Number(value ?? 0)
  try {
    await cart.setQty(productId, qty)
    if (qty <= 0) feedback.info('已移出购物车')
  } catch (err) {
    feedback.fromError(err)
    await cart.load().catch(() => {})
  }
}

async function remove(productId: number, name: string): Promise<void> {
  try {
    await cart.remove(productId)
    feedback.success('已移出购物车', name)
  } catch (err) {
    feedback.fromError(err)
  }
}

async function clearAll(): Promise<void> {
  const ok = await feedback.confirm(`将清空购物车中的 ${cart.count} 件商品，确定继续？`, '清空购物车', '确定清空')
  if (!ok) return
  try {
    await cart.clear()
    feedback.success('购物车已清空')
  } catch (err) {
    feedback.fromError(err)
  }
}
</script>

<template>
  <div class="app-page">
    <el-row :gutter="12">
      <el-col :xs="24" :lg="18">
        <div class="card">
          <div class="toolbar">
            <el-button :loading="cart.loading" @click="cart.load()">
              <el-icon><Refresh /></el-icon>刷新
            </el-button>
            <el-button type="danger" plain :disabled="rows.length === 0" @click="clearAll">
              <el-icon><Delete /></el-icon>清空购物车
            </el-button>
            <div class="toolbar__fill" />
            <span class="text-soft">共 {{ cart.badgeCount }} 种 · {{ cart.count }} 件</span>
          </div>

          <div class="card__bd card__bd--flush">
            <el-table v-loading="cart.loading" :data="rows" style="width: 100%">
              <template #empty>
                <el-empty image-size="70" description="购物车是空的">
                  <el-button type="primary" @click="router.push('/goods/list')">去商品管理挑一个</el-button>
                </el-empty>
              </template>

              <!-- 来源并进商品单元格：固定操作列会压住最右侧的独立来源列 -->
              <el-table-column label="商品" min-width="250">
                <template #default="{ row }">
                  <div class="line">
                    <CoverPlate :src="row.cover" :name="row.productName" size="sm" />
                    <div class="line__meta">
                      <span class="line__name">
                        <span class="line__text">{{ row.productName || `商品 #${row.productId}` }}</span>
                        <el-tag
                          v-if="row.activityId > 0"
                          type="success"
                          size="small"
                          effect="light"
                        >
                          秒杀
                        </el-tag>
                        <el-tag v-else type="info" size="small" effect="light">常规</el-tag>
                      </span>
                      <span class="mono line__pid">PID {{ row.productId }} · 活动 {{ row.activityId || '—' }}</span>
                    </div>
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="单价" width="150">
                <template #default="{ row }">
                  <div class="mono price">{{ money(row.price) }}</div>
                  <div v-if="row.originalPrice > row.price" class="mono text-strike was">
                    {{ money(row.originalPrice) }}
                  </div>
                </template>
              </el-table-column>

              <el-table-column label="数量" width="150">
                <template #default="{ row }">
                  <el-input-number
                    :model-value="row.qty"
                    :min="0"
                    :max="99"
                    size="small"
                    :disabled="cart.busyId === row.productId"
                    @change="(v: number | undefined) => changeQty(row.productId, v)"
                  />
                </template>
              </el-table-column>

              <el-table-column label="小计" width="130" align="right">
                <template #default="{ row }">
                  <span class="mono subtotal">{{ money(row.price * row.qty) }}</span>
                </template>
              </el-table-column>

              <el-table-column label="操作" width="90" fixed="right">
                <template #default="{ row }">
                  <el-button link type="danger" :disabled="cart.busyId === row.productId" @click="remove(row.productId, row.productName)">
                    移除
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </div>
      </el-col>

      <el-col :xs="24" :lg="6">
        <div class="card">
          <div class="card__hd"><span class="card__title">结算汇总</span></div>
          <div class="card__bd">
            <div class="sum">
              <span class="text-soft">种类 / 件数</span>
              <span class="mono">{{ cart.badgeCount }} / {{ cart.count }}</span>
            </div>
            <div class="sum">
              <span class="text-soft">合计</span>
              <span class="mono total">{{ money(cart.subtotal) }}</span>
            </div>
            <div v-if="cart.saved > 0" class="sum">
              <span class="text-soft">比原价省下</span>
              <span class="mono text-green">{{ money(cart.saved) }}</span>
            </div>

            <el-divider />

            <el-button type="primary" style="width: 100%" @click="router.push('/order/list')">
              <el-icon><Tickets /></el-icon>查看订单
            </el-button>

            <el-alert
              class="tip"
              type="info"
              :closable="false"
              show-icon
              title="购物车存在 Redis"
              description="真正占住库存的是下单那一刻的 Lua 原子扣减，购物车只负责暂存。"
            />
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.line {
  display: flex;
  align-items: center;
  gap: 8px;
}

.line__meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.line__name {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.line__text {
  color: var(--ink-900);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.line__pid {
  font-size: 11px;
  color: var(--ink-400);
}

.price {
  color: var(--ink-900);
  font-weight: 500;
}

.was {
  font-size: 12px;
}

.subtotal {
  font-weight: 600;
  color: var(--brand-600);
}

.sum {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 5px 0;
  font-size: 13px;
}

.total {
  font-size: 20px;
  font-weight: 600;
  color: var(--brand-600);
}

.tip {
  margin-top: 14px;
}
</style>
