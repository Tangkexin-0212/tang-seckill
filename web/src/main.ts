/* 数字/编号用等宽（ID、订单号、路径、时间戳），中文走系统无衬线栈 */
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/500.css'
import '@fontsource/ibm-plex-mono/600.css'

/* 顺序不能变：先 EP 原始样式，再我们的变量覆盖层，最后是原子类 */
import 'element-plus/dist/index.css'
import './styles/tokens.css'
import './styles/element.css'
import './styles/base.css'

import * as Icons from '@element-plus/icons-vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { createPinia } from 'pinia'
import { createApp, watch } from 'vue'
import { setFatalHandler, setTokenProvider, setUnauthorizedHandler } from './api/client'
import App from './App.vue'
import { router } from './router'
import { useAuth } from './stores/auth'
import { useCart } from './stores/cart'
import { feedback } from './utils/feedback'

/** 只注册实际用到的图标，避免整包注册把产物撑大 */
const ICONS = [
  'Odometer',
  'Timer',
  'List',
  'Pointer',
  'Goods',
  'Tickets',
  'ShoppingCart',
  'Tools',
  'Search',
  'Refresh',
  'RefreshRight',
  'FullScreen',
  'Fold',
  'Expand',
  'Close',
  'Plus',
  'Delete',
  'Edit',
  'CopyDocument',
  'Download',
  'Upload',
  'DataLine',
  'Bell',
  'User',
  'SwitchButton',
  'Setting',
  'Link',
  'View',
  'CircleCheck',
  'CircleClose',
  'Warning',
  'WarningFilled',
  'InfoFilled',
  'SuccessFilled',
  'ArrowDown',
  'ArrowRight',
  'ArrowLeft',
  'Calendar',
  'Wallet',
  'TrendCharts',
  'Monitor',
  'Connection',
  'Check',
  'Money',
  'Ticket',
  'Box',
  'HomeFilled',
  'Cpu',
  'Histogram',
  'Right',
] as const

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(ElementPlus, { locale: zhCn })

for (const name of ICONS) {
  const icon = (Icons as Record<string, unknown>)[name]
  if (icon) app.component(name, icon)
}

/* api 层不 import store（避免循环引用），依赖在这里注入 */
const auth = useAuth(pinia)
const cart = useCart(pinia)

setTokenProvider(() => auth.token || null)

setUnauthorizedHandler(() => {
  const hadToken = auth.token !== ''
  auth.clear()
  if (!hadToken) return
  feedback.warning('登录态已失效', 'Token 过期或无效，请重新登录。')
  if (router.currentRoute.value.name !== 'login') {
    const redirect = router.currentRoute.value.fullPath
    void router.push({ name: 'login', query: redirect && redirect !== '/' ? { redirect } : {} })
  }
})

/* 20001 / 20002 是后端真的出问题了，必须显性告知 */
setFatalHandler((err) => feedback.fromError(err))

/* 必须在 mount 之前注册：否则 RouterView 拿不到 router 注入，整页白屏 */
app.use(router)

watch(
  () => auth.isLoggedIn,
  (alive) => {
    if (alive) {
      // 只取条目数（HLen），不必为了一个角标把整个购物车拉回浏览器
      void cart.loadCount()
    } else {
      cart.reset()
    }
  },
  { immediate: true },
)

app.mount('#app')
