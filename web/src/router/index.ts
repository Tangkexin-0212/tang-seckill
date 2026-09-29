import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import AdminLayout from '@/layouts/AdminLayout.vue'
import { useAuth } from '@/stores/auth'

/**
 * 路由即菜单：meta.title 用于面包屑与标签页，meta.icon 用于侧边菜单，
 * meta.hidden 的页面有路由但不进菜单（如带参数的抢购台）。
 * requiresAuth 只保护个人数据页；仪表盘/活动/商品都是公开只读接口，不拦。
 */
const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { title: '登录', hidden: true },
  },
  {
    path: '/',
    component: AdminLayout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('@/views/DashboardView.vue'),
        meta: { title: '仪表盘', icon: 'Odometer', affix: true },
      },
      {
        path: 'seckill',
        redirect: '/seckill/activity',
        meta: { title: '秒杀管理', icon: 'Timer' },
        children: [
          {
            path: 'activity',
            name: 'activityList',
            component: () => import('@/views/seckill/ActivityListView.vue'),
            meta: { title: '活动列表', icon: 'List' },
          },
          {
            path: 'grab/:id',
            name: 'grab',
            props: true,
            component: () => import('@/views/seckill/GrabView.vue'),
            meta: { title: '抢购台', icon: 'Pointer', hidden: true },
          },
        ],
      },
      {
        path: 'goods/list',
        name: 'goodsList',
        component: () => import('@/views/goods/GoodsListView.vue'),
        meta: { title: '商品管理', icon: 'Goods' },
      },
      {
        path: 'order/list',
        name: 'orderList',
        component: () => import('@/views/order/OrderListView.vue'),
        meta: { title: '订单管理', icon: 'Tickets', requiresAuth: true },
      },
      {
        path: 'cart/list',
        name: 'cartList',
        component: () => import('@/views/cart/CartListView.vue'),
        meta: { title: '购物车', icon: 'ShoppingCart', requiresAuth: true },
      },
      {
        path: 'system/tool',
        name: 'tool',
        component: () => import('@/views/system/ToolView.vue'),
        meta: { title: '系统工具', icon: 'Tools', requiresAuth: true },
      },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach((to) => {
  const auth = useAuth()
  if (to.meta.requiresAuth && !auth.isLoggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  return true
})

router.afterEach((to) => {
  const title = (to.meta.title as string | undefined) ?? ''
  document.title = title ? `${title} - 秒杀管理台` : '秒杀管理台'
})
