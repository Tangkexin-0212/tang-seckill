<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { adminApi, catalogApi } from '@/api'
import type { Product } from '@/api/types'
import CoverPlate from '@/components/CoverPlate.vue'
import { useCart } from '@/stores/cart'
import { feedback } from '@/utils/feedback'
import { money } from '@/utils/format'

const cart = useCart()

const loading = ref(false)
const products = ref<Product[]>([])

const query = reactive({
  name: '',
  minYuan: undefined as number | undefined,
  maxYuan: undefined as number | undefined,
  status: '' as '' | '1' | '0',
})
const page = ref(1)
const pageSize = ref(10)

async function load(): Promise<void> {
  loading.value = true
  try {
    products.value = (await catalogApi.products(100, 0)) ?? []
  } catch (err) {
    feedback.fromError(err)
  } finally {
    loading.value = false
  }
}

onMounted(load)

const filtered = computed(() =>
  products.value.filter((p) => {
    if (query.name.trim() && !p.name.toLowerCase().includes(query.name.trim().toLowerCase())) return false
    if (query.minYuan !== undefined && p.price < Math.round(query.minYuan * 100)) return false
    if (query.maxYuan !== undefined && p.price > Math.round(query.maxYuan * 100)) return false
    if (query.status !== '' && String(p.status) !== query.status) return false
    return true
  }),
)

const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

watch([filtered, pageSize], () => {
  const maxPage = Math.max(1, Math.ceil(filtered.value.length / pageSize.value))
  if (page.value > maxPage) page.value = maxPage
})

function resetQuery(): void {
  query.name = ''
  query.minYuan = undefined
  query.maxYuan = undefined
  query.status = ''
  page.value = 1
}

/* ---------------- 加入购物车 ---------------- */
async function addToCart(p: Product): Promise<void> {
  try {
    await cart.add(p.id, 1)
    feedback.success('已加入购物车', p.name)
  } catch (err) {
    feedback.fromError(err)
  }
}

/* ---------------- 新增商品 ---------------- */
const dialogVisible = ref(false)
const submitting = ref(false)
const form = reactive({ name: '', priceYuan: 1999, cover: '', description: '' })

function openCreate(): void {
  form.name = ''
  form.priceYuan = 1999
  form.cover = ''
  form.description = ''
  dialogVisible.value = true
}

async function submitCreate(): Promise<void> {
  if (!form.name.trim()) {
    feedback.warning('商品名不能为空')
    return
  }
  if (form.priceYuan <= 0) {
    feedback.warning('价格必须大于 0')
    return
  }

  submitting.value = true
  try {
    const res = await adminApi.createProduct({
      name: form.name.trim(),
      price: Math.round(form.priceYuan * 100),
      cover: form.cover.trim(),
      description: form.description.trim(),
    })
    feedback.success('商品已创建', `ID ${res.productId}`)
    dialogVisible.value = false
    await load()
  } catch (err) {
    feedback.fromError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="app-page">
    <div class="card">
      <div class="card__bd">
        <div class="search-bar">
          <el-form inline @submit.prevent>
            <el-form-item label="商品名称">
              <el-input v-model="query.name" placeholder="模糊匹配" clearable style="width: 200px" />
            </el-form-item>
            <el-form-item label="价格区间（元）">
              <el-input v-model.number="query.minYuan" placeholder="最低" clearable style="width: 96px" />
              <span class="range-sep">-</span>
              <el-input v-model.number="query.maxYuan" placeholder="最高" clearable style="width: 96px" />
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="query.status" placeholder="全部" clearable style="width: 120px">
                <el-option label="上架" value="1" />
                <el-option label="下架" value="0" />
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
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>新增商品</el-button>
        <el-button :loading="loading" @click="load"><el-icon><Refresh /></el-icon>刷新</el-button>
        <div class="toolbar__fill" />
        <span class="text-soft">共 {{ filtered.length }} 条</span>
      </div>

      <div class="card__bd card__bd--flush">
        <el-table v-loading="loading" :data="paged" style="width: 100%">
          <template #empty>
            <el-empty image-size="70" description="商品库是空的，点「新增商品」建一个" />
          </template>

          <el-table-column prop="id" label="ID" width="80" sortable>
            <template #default="{ row }: { row: Product }">
              <span class="mono text-brand">#{{ row.id }}</span>
            </template>
          </el-table-column>

          <el-table-column label="商品" min-width="220">
            <template #default="{ row }: { row: Product }">
              <div class="goods">
                <CoverPlate :src="row.cover" :name="row.name" size="sm" />
                <span class="goods__name">{{ row.name }}</span>
              </div>
            </template>
          </el-table-column>

          <el-table-column prop="price" label="价格" width="130" sortable align="right">
            <template #default="{ row }: { row: Product }">
              <span class="mono price">{{ money(row.price) }}</span>
            </template>
          </el-table-column>

          <el-table-column prop="description" label="描述" min-width="200" show-overflow-tooltip>
            <template #default="{ row }: { row: Product }">
              <span :class="{ 'text-soft': !row.description }">{{ row.description || '—' }}</span>
            </template>
          </el-table-column>

          <el-table-column label="封面地址" width="180" show-overflow-tooltip>
            <template #default="{ row }: { row: Product }">
              <span class="mono text-soft">{{ row.cover || '未设置' }}</span>
            </template>
          </el-table-column>

          <el-table-column label="状态" width="100">
            <template #default="{ row }: { row: Product }">
              <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
                {{ row.status === 1 ? '上架' : '下架' }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }: { row: Product }">
              <div class="op-links">
                <el-button
                  link
                  type="primary"
                  :disabled="row.status !== 1"
                  :loading="cart.busyId === row.id"
                  @click="addToCart(row)"
                >
                  <el-icon><ShoppingCart /></el-icon>加入购物车
                </el-button>
                <el-button link type="primary" @click="feedback.copy(String(row.id), '商品 ID')">复制 ID</el-button>
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

    <el-dialog v-model="dialogVisible" title="新增商品" width="520px" append-to-body>
      <el-form label-width="96px" @submit.prevent>
        <el-form-item label="商品名称" required>
          <el-input v-model="form.name" placeholder="例如：夜场机械键盘" />
        </el-form-item>
        <el-form-item label="价格（元）" required>
          <el-input-number v-model="form.priceYuan" :min="0.01" :precision="2" :step="100" style="width: 180px" />
        </el-form-item>
        <el-form-item label="封面 URL">
          <el-input v-model="form.cover" placeholder="留空则显示占位牌，可填任意 http(s) 地址" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="可选" />
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
.range-sep {
  margin: 0 6px;
  color: var(--ink-400);
}

.goods {
  display: flex;
  align-items: center;
  gap: 8px;
}

.goods__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ink-900);
}

.price {
  color: var(--ink-900);
  font-weight: 600;
}

.pager {
  padding: 0 16px 14px;
}
</style>
