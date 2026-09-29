<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError, CODE } from '@/api/client'
import { useAuth } from '@/stores/auth'
import { feedback } from '@/utils/feedback'
import { duration } from '@/utils/format'

const auth = useAuth()
const route = useRoute()
const router = useRouter()

type Mode = 'login' | 'register'
const mode = ref<Mode>('login')

const form = reactive({ username: '', password: '' })
const submitting = ref(false)
const errorText = ref('')

const redirect = computed(() => {
  const r = route.query.redirect
  return typeof r === 'string' && r.startsWith('/') ? r : '/dashboard'
})

function switchMode(next: Mode): void {
  mode.value = next
  errorText.value = ''
}

/** 与后端 user 服务的校验规则保持一致：用户名 >= 3，密码 >= 6 */
function validate(): boolean {
  if (form.username.trim().length < 3) {
    errorText.value = '用户名至少 3 个字符'
    return false
  }
  if (form.password.length < 6) {
    errorText.value = '密码至少 6 位'
    return false
  }
  return true
}

async function submit(): Promise<void> {
  if (submitting.value) return
  errorText.value = ''
  if (!validate()) return

  submitting.value = true
  const name = form.username.trim()
  try {
    // 注册后直接登录，只给一条提示——分两条是噪音，用户关心的是"进没进去"
    const isRegister = mode.value === 'register'
    if (isRegister) await auth.register(name, form.password)
    await auth.login(name, form.password)

    feedback.success(isRegister ? '号牌已领取，已进场' : '登录成功', `UID ${auth.uid}`)
    await router.replace(redirect.value)
  } catch (err) {
    const e = err instanceof ApiError ? err : new ApiError(0, String(err))
    if (e.code === CODE.UNAUTHORIZED) errorText.value = '用户名或密码不正确。'
    else if (e.code === CODE.BAD_REQUEST) errorText.value = e.message || '参数不合法：用户名至少 3 位、密码至少 6 位。'
    else {
      errorText.value = e.message || '登录失败，请稍后再试。'
      feedback.fromError(err)
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="login">
    <div class="login__card">
      <!-- 左：品牌与链路说明 -->
      <aside class="login__brand">
        <div class="brand__logo">
          <svg viewBox="0 0 32 32" width="34" height="34" aria-hidden="true">
            <rect x="1" y="1" width="30" height="30" rx="9" fill="rgba(255,255,255,.18)" />
            <path d="M13 8h6l-2.2 6H20l-8 10 2.2-7H10z" fill="#fff" />
          </svg>
        </div>
        <h1 class="brand__title">秒杀管理台</h1>
        <p class="brand__sub">SECKILL ADMIN · Kitex + Hertz + Redis Lua</p>

        <ul class="brand__list">
          <li><span>库存</span>Redis 为权威，MySQL 最终一致</li>
          <li><span>限流</span>用户 / 活动 / 全局 三层令牌桶</li>
          <li><span>下单</span>Lua 原子扣减 + 唯一索引兜底幂等</li>
          <li><span>落库</span>Stream → worker 异步消费，at-least-once</li>
        </ul>
      </aside>

      <!-- 右：表单 -->
      <section class="login__form">
        <h2 class="form__title">{{ mode === 'login' ? '登录' : '注册' }}</h2>
        <p class="form__desc">
          {{ mode === 'login' ? '使用账号密码进入管理台' : '用户名至少 3 位，密码至少 6 位' }}
        </p>

        <template v-if="auth.isLoggedIn">
          <el-alert
            type="success"
            :closable="false"
            show-icon
            title="你已经登录了"
            :description="`UID ${auth.uid} · 号牌剩余 ${duration(auth.msLeft)}`"
          />
          <div class="form__actions">
            <el-button type="primary" style="width: 100%" @click="router.push(redirect)">继续</el-button>
            <el-button style="width: 100%" @click="auth.clear()">换一个账号</el-button>
          </div>
        </template>

        <template v-else>
          <el-tabs v-model="mode" class="form__tabs" @tab-change="(n: string | number) => switchMode(n as Mode)">
            <el-tab-pane label="登录" name="login" />
            <el-tab-pane label="注册" name="register" />
          </el-tabs>

          <el-form label-position="top" @submit.prevent="submit">
            <el-form-item label="用户名">
              <el-input
                v-model="form.username"
                size="large"
                placeholder="至少 3 个字符"
                :disabled="submitting"
                clearable
              >
                <template #prefix><el-icon><User /></el-icon></template>
              </el-input>
            </el-form-item>

            <el-form-item label="密码">
              <el-input
                v-model="form.password"
                type="password"
                size="large"
                placeholder="至少 6 位"
                :disabled="submitting"
                show-password
                @keyup.enter="submit"
              >
                <template #prefix><el-icon><Setting /></el-icon></template>
              </el-input>
            </el-form-item>

            <el-alert v-if="errorText" class="form__error" type="error" :closable="false" show-icon :title="errorText" />

            <el-button
              class="form__submit"
              type="primary"
              size="large"
              :loading="submitting"
              @click="submit"
            >
              {{ submitting ? '校验中…' : mode === 'login' ? '登录' : '注册并登录' }}
            </el-button>
          </el-form>

          <p class="form__foot">
            {{ mode === 'login' ? '还没有账号？' : '已经有账号了？' }}
            <el-button link type="primary" @click="switchMode(mode === 'login' ? 'register' : 'login')">
              {{ mode === 'login' ? '去注册' : '去登录' }}
            </el-button>
          </p>
        </template>
      </section>
    </div>

    <p class="login__foot">Powered by 秒杀管理台 · 网关 127.0.0.1:8080</p>
  </div>
</template>

<style scoped>
.login {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  /* 极浅的蓝绿氛围底：没有它就只是一个白板，有了它才是同一套视觉体系 */
  background:
    radial-gradient(900px 500px at 12% -10%, rgba(22, 119, 255, 0.1), transparent 60%),
    radial-gradient(800px 500px at 92% 110%, rgba(24, 160, 88, 0.12), transparent 60%),
    var(--bg-page);
}

.login__card {
  display: flex;
  width: 100%;
  max-width: 900px;
  min-height: 480px;
  background: #fff;
  border: 1px solid var(--line);
  border-radius: 10px;
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}

/* ---- 左侧品牌面 ---- */
.login__brand {
  flex: 0 0 46%;
  padding: 44px 40px;
  color: #fff;
  background: linear-gradient(150deg, #1677ff 0%, #1a9be0 46%, #18a058 100%);
  display: flex;
  flex-direction: column;
}

.brand__title {
  margin-top: 18px;
  font-size: 26px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.brand__sub {
  margin-top: 6px;
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  opacity: 0.82;
}

.brand__list {
  list-style: none;
  padding: 0;
  margin: 34px 0 0;
  font-size: 12.5px;
  line-height: 1.9;
}

.brand__list li {
  padding: 7px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.16);
  opacity: 0.95;
}

.brand__list span {
  display: inline-block;
  width: 42px;
  margin-right: 8px;
  font-weight: 600;
  opacity: 0.85;
}

/* ---- 右侧表单面 ---- */
.login__form {
  flex: 1;
  padding: 44px 40px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.form__title {
  font-size: 22px;
  font-weight: 600;
  color: var(--ink-900);
}

.form__desc {
  margin-top: 6px;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--ink-500);
}

.form__tabs {
  margin-bottom: 6px;
}

.form__tabs :deep(.el-tabs__header) {
  margin-bottom: 14px;
}

.form__tabs :deep(.el-tabs__item) {
  font-size: 14px;
  padding: 0 16px 0 0;
}

.form__error {
  margin-bottom: 12px;
}

.form__submit {
  width: 100%;
  margin-top: 4px;
}

.form__actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 18px;
}

.form__actions .el-button {
  margin-left: 0;
}

.form__foot {
  margin-top: 16px;
  font-size: 13px;
  color: var(--ink-500);
}

.login__foot {
  margin-top: 18px;
  font-size: 12px;
  color: var(--ink-400);
}

@media (max-width: 820px) {
  .login__card {
    flex-direction: column;
  }

  .login__brand {
    flex: none;
    padding: 28px 24px;
  }

  .brand__list {
    display: none;
  }

  .login__form {
    padding: 28px 24px;
  }
}
</style>
