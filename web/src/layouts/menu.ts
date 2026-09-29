/**
 * 侧边菜单配置。
 *
 * 刻意不写成"从 router 自动生成"：菜单顺序、分组、图标是产品决策，
 * 路由表是技术结构，两者混在一起后每次加路由都得反过来操心菜单排布。
 * 侧边栏与顶栏的"菜单搜索"共用这一份配置，保证两边永远一致。
 */
export interface MenuEntry {
  title: string
  icon: string
  path?: string
  children?: MenuEntry[]
}

export const MENUS: MenuEntry[] = [
  { title: '仪表盘', icon: 'Odometer', path: '/dashboard' },
  {
    title: '秒杀管理',
    icon: 'Timer',
    children: [{ title: '活动列表', icon: 'List', path: '/seckill/activity' }],
  },
  { title: '商品管理', icon: 'Goods', path: '/goods/list' },
  { title: '订单管理', icon: 'Tickets', path: '/order/list' },
  { title: '购物车', icon: 'ShoppingCart', path: '/cart/list' },
  { title: '系统工具', icon: 'Tools', path: '/system/tool' },
]

/** 拍平成"可跳转项"清单，供顶栏搜索使用 */
export function flatMenus(menus: MenuEntry[] = MENUS): Array<{ title: string; path: string; icon: string; group: string }> {
  const out: Array<{ title: string; path: string; icon: string; group: string }> = []
  for (const m of menus) {
    if (m.children?.length) {
      for (const c of m.children) {
        if (c.path) out.push({ title: c.title, path: c.path, icon: c.icon, group: m.title })
      }
    } else if (m.path) {
      out.push({ title: m.title, path: m.path, icon: m.icon, group: '' })
    }
  }
  return out
}
