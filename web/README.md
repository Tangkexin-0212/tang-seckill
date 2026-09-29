# 秒杀管理台 · 前端

对接 `gateway`（Hertz，`127.0.0.1:8080`）的秒杀系统管理台。**Vue 3 + Vite + TypeScript + Element Plus**。

视觉基线参考 gin-vue-admin：白底 + 左侧分组导航 + 顶栏面包屑 + 可关闭标签页 + 表格工作区，
主色为白/蓝/绿（蓝=导航选中与主按钮，绿=成功态与库存，白=数据承载面）。

## 运行

```powershell
# 推荐：一条命令，会先检查 gateway 可达性再起 Vite
powershell -ExecutionPolicy Bypass -File ..\scripts\start-web.ps1

# 或连后端一起拉起来
powershell -ExecutionPolicy Bypass -File ..\scripts\start-all.ps1 -SkipInfra -SkipBuild -WithWeb

# 手工方式
npm install
npm run dev        # http://127.0.0.1:5173
npm run build      # vue-tsc -b && vite build
npm run preview    # http://127.0.0.1:4173（同样挂了 /api 代理）
npm run typecheck  # vue-tsc -b
```

## 为什么必须走代理

`gateway` 既没有 CORS 响应头，也不托管静态资源。前端若直接 `fetch('http://127.0.0.1:8080/api/...')` 会被浏览器跨域拦截。

所以 `vite.config.ts` 给 **dev 与 preview 都**挂了同源反代：

```
/api    → http://127.0.0.1:8080
/health → http://127.0.0.1:8080
```

浏览器只看到 `127.0.0.1:5173/api/*`，跨域问题在根上不存在，**后端一行代码都不用改**。
后端地址可用环境变量覆盖：`SK_BACKEND=http://other:8080 npm run dev`。

## 必须记住的接口事实

网关**永远返回 HTTP 200**，成败只能看响应体里的 `code`：

- `src/api/client.ts` 是唯一请求入口，只有 `code === 0` 才返回 `data`，否则抛 `ApiError{code,msg}`；
- 例外：`/health` 直接把结果放在响应包上（无 `data` 字段），走 `getRaw` 而不是 `get`；
- `code=10005` 触发全局登出并跳登录；`code=20001/20002` 触发全局告警通知，绝不静默；
- `code=10001~10010`（限流/已抢过/售罄）是"用户在秒杀里正常会撞上的墙"，
  用 `ElMessage` 的 info/warning 语气，不当作系统故障。

错误码 → 界面表现的映射集中在 `src/api/errors.ts`，再经 `src/utils/feedback.ts` 接到 Element Plus 的消息组件上。改文案只改这两处。

## 购物车有两种口径，别混

| 口径 | 含义 | 来源 | 用在哪 |
| --- | --- | --- | --- |
| **种数 / 条目数** | 购物车里有几个不同商品 | `GET /api/cart/count`（服务端 `HLen`，O(1)） | 侧栏角标、顶栏信息面板、仪表盘 |
| **件数** | `qty` 之和 | `GET /api/cart/list` 的明细求和 | 购物车页的"共 N 件"、结算汇总 |

买了 3 件同一个商品时，种数是 1、件数是 3。角标走 `/api/cart/count` 而不是拉全量列表，
是因为它只需要一个整数，没必要把 `sk:cart:{uid}` 与 `sk:cart:snap:{uid}` 两个 Hash 全量搬回浏览器。
`stores/cart.ts` 里两者分别对应 `badgeCount` 与 `count`，界面上一律同时标注口径，避免数字互相打脸。

## 目录

```
src/
  api/          client.ts（请求封装 + 错误码）· types.ts（与 IDL 对应）· errors.ts（码→语气映射）
  layouts/      AdminLayout.vue（侧栏 + 顶栏 + 标签页）· menu.ts（菜单配置，侧栏与菜单搜索共用）
                components/SideMenu.vue · NavBar.vue · TagsView.vue
  composables/  useServerClock.ts（全局时钟 + 半程时延补偿）
  components/   CoverPlate.vue（商品图位，破图自动降级为占位牌）
  stores/       auth（Token 持久化）· cart（Redis 购物车本地镜像）· app（侧栏折叠/刷新令牌）· tags（标签页）
  styles/       tokens.css（设计令牌）→ element.css（Element Plus 变量覆盖）→ base.css（原子类）
  utils/        feedback.ts（统一反馈出口）· format.ts（分→元、时长、时间）· status.ts（状态→el-tag）
  views/        DashboardView · LoginView
                seckill/ActivityListView · seckill/GrabView
                goods/GoodsListView · order/OrderListView · cart/CartListView · system/ToolView
```

## 页面

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| `/dashboard` | 仪表盘 | 4 张指标卡 + 最近订单 + 系统状态 + 去化 Top5 + 在槌速览 |
| `/seckill/activity` | 活动列表 | 查询条 + 工具栏 + 表格（分页/排序）+ 新增活动对话框 + 批量预热 |
| `/seckill/grab/:id` | 抢购台 | 倒计时 + 库存 + 抢购状态机 + 落槌结果；不在菜单里，供活动列表进入 |
| `/goods/list` | 商品管理 | 查询 + 表格 + 新增商品 + 加入购物车 |
| `/order/list` | 订单管理 | 订单表格 + 支付 / 放弃 + 剩余支付时间进度 |
| `/cart/list` | 购物车 | 数量增减 + 移除 + 清空 + 结算汇总 |
| `/system/tool` | 系统工具 | 一键开一场 / 分步建商品·建活动·预热 / 链路自检 / 控制台输出 |
| `/login` | 登录注册 | 左右分栏卡片（左侧蓝绿渐变品牌面 + 右侧表单） |

## 几个刻意的取舍

1. **不轮询库存**。只在进入抢购台、抢购前后各拉一次 `/api/seckill/info`，倒计时归零时自动同步一次。高频轮询会打后端，也会让"秒杀"这件事显得廉价。
2. **时钟只做半程补偿**。响应体里没有服务器时间戳，无法真正跨机对齐时钟，所以只消掉"响应在路上的 rtt/2"这个系统性滞后（上限 500ms），并把这件事写进代码注释。
3. **破图必降级**。示例数据的 `cover` 大量是 `https://example.com/...` 或 `"x"`，`CoverPlate` 只在 URL 合法时真加载，失败即降级为程序化占位牌，界面上永远不会出现破图。
4. **表格不堆列**。活动列表把"秒杀价"并进商品单元格，让 6 列能在 1080 宽度下完整显示——12 列挤在一起再宽的屏幕也读不动。
5. **菜单是配置而不是路由生成**。菜单顺序/分组是产品决策，路由表是技术结构；`layouts/menu.ts` 一份配置同时供侧栏和顶栏"菜单搜索"使用。
