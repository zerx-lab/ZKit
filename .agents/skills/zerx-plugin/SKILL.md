---
name: zerx-plugin
description: "ZKit 编译期插件机制:以低侵入方式新增后台管理模块(后端 RPC + 表 + 前端动态页 + 菜单 + 授权),不破坏单二进制 / CGO-free / Casbin 唯一授权 / proto 唯一契约。当用户要用插件方式扩展、用 task new-plugin 脚手架、新增/启用/禁用/卸载插件、或排查插件注册/校验/动态前端页问题时使用。Keywords: plugin, 插件, plugin system, 插件机制, task new-plugin, task plugin-pack, 脚手架, scaffold, 上传插件, 安装插件, 卸载插件, install plugin, uninstall plugin, upload, zip, InstallPlugin, UninstallPlugin, internal/plugin, installer, all.go, ValidateAll, RegisterHandlers, 动态路由, import.meta.glob, p.$, pub.$, plugin-components, 编译期插件, compile-time plugin, 插件注册, 插件禁用, 插件卸载, teardown, PLUGIN_UPLOAD_ENABLED"
---
# ZKit 编译期插件机制

> 编译期静态插件(源码内置 + 重新构建):自带 RPC/表/迁移/菜单/授权声明/可选 job;显式 `Register()` 注册;启动期 `ValidateAll` 强校验;前端单一动态路由按 `menu.component` 加载页。**有意不用** WASM/.so/go-plugin/init() 魔法(理由见设计裁决)。

## 加一个插件(脚手架,推荐)
```
task new-plugin -- <name> [field:type,...]   # 例: task new-plugin -- blog title:string,views:int(shop 已作为内置示例)
task gen                                       # 生成 proto Go/TS + connect-query hook
# 填 internal/plugin/impl/<name>/service.go 业务逻辑(脚手架已给 CRUD stub)
task build && 重启
```
- `name` 须匹配 `^[a-z][a-z0-9_]{0,30}$`。字段类型:`string|int|int32|bool|float`;`id`/`name` 为内置字段,传入会被忽略(避免重复)。
- 生成器产出:`proto/zerx/v1/<name>.proto`、`internal/plugin/impl/<name>/{plugin,model,service}.go`、`<name>_teardown.sql`、`web/src/plugin-components/<name>/<Pascal>.tsx`,并在 `internal/plugins/all.go` 两处锚点插行(`// plugin-import-anchor` / `// plugin-register-anchor`)后 gofmt。

## 手动新增插件的核心改动 = 仅 all.go 两行
装配点(server.New / main.go)各只接一次钩子,之后永不再改。新增插件**唯一**核心改动是 `internal/plugins/all.go`:import 块加一行 + `Register()` 体加一行。

## Plugin 契约(`internal/plugin/plugin.go`)
实现 `plugin.Plugin` 接口(示例见 `internal/plugin/impl/shop/plugin.go`):
- `Name()` 小写蛇形,全局唯一。
- `Services()` 本插件 connectRPC service 全名(如 `zerx.v1.ShopProductService`)——`ValidateAll` 据此校验 public/selfServe procedure 归属。
- `Migrations()` gormigrate 迁移,ID 须 `plg_<name>_` 前缀;`TableNames()` 表名须 `plg_<name>_` 前缀。
- `SeedMenus()` 菜单 `Name` 须 `plg_<name>` 前缀、`Path` 须 `/p/<name>` 前缀;`Component` 为 glob 相对标识(如 `shop/Shop`);`UserVisible` 控制是否默认授 user 角色 RoleMenu。
- `PublicProcedures()/SelfServeProcedures()` 必属本插件 service(否则启动 fatal,堵提权)。
- `RegisterHandlers(reg, deps)` 用 `deps.Opts`(完整拦截器链)注册 handler;`deps` 含 `DB/Opts/Enforcer/Media/Logger`。
- `JobHandlers()` 可选,key 须 `<name>_` 前缀;类型是 `plugin.JobHandler{Run,Description}`(**不引 jobs.Descriptor**,断开循环引用)。

## 启动期校验(`ValidateAll`,失败 fatal)
main.go 在 `plugins.Register()` 后、任何 DB 操作前调用,校验:名字正则+唯一、迁移ID前缀、表名前缀、job key 前缀、**Service 名以插件 PascalCase 前缀且全局唯一**、**public/selfServe procedure 必属本插件 service**、菜单 Name/Path 命名空间且与核心 seed 及其他插件无冲突、public page 路径 `/pub/<name>` 命名空间。
- **提权防线(安全核)**:`Services()` 每项 short name 必须以插件 PascalCase 名开头(`shop` → `ShopProductService`),且一个 service 全局只能属一个插件。这闸死了"插件声明拥有核心 service(如 `zerx.v1.UserService`)再把其 procedure 洗进 public/selfServe map"的提权通道——否则 procedure-ownership 校验会被架空。
- **已知边界(信任模型,见设计 Assumption A)**:`TableNames()` 是插件**自声明**列表,ValidateAll 校验的是该声明的前缀,**不内省迁移实际建的表**。插件被定位为同仓库可重建二进制的可信协作方;若迁移里 `AutoMigrate` 一张非前缀(甚至核心)表,ValidateAll 不拦——靠 code review / CI 把关。务必让 model 的 `TableName()` 返回 `plg_<name>_*` 且与 `TableNames()` 一致(示例 shop 已如此)。

## 授权(Casbin 唯一权威,零拦截器改动)
插件 handler 经同一 `opts` 注册 → 自动受 Casbin 保护。插件 procedure 默认仅 admin 可用(admin bypass)。让非 admin 角色用上插件页需管理员在 UI 配两类:RoleMenu(菜单可见)+ RolePermission/casbin_rule(接口放行)。**插件不自行 seed casbin_rule 提权**。细节 → `skill://zerx-authz`。

## 前端动态页(承重机制)
- 页放 `web/src/plugin-components/<name>/<Component>.tsx`(**default export**)。
- 单一 splat 路由 `web/src/routes/_authed/p.$.tsx`(参数 `_splat`):`import.meta.glob('/src/plugin-components/**/*.tsx')` 编译期收集;用 `path==='/p/'+_splat` 在用户菜单树找到 `menu.component`,拼 key `/src/plugin-components/${component}.tsx`,`React.lazy` 加载;找不到渲染 NotFound。splat 同时支持扁平 `/p/<name>` 与分组子页 `/p/<name>/<sub>`。
- **分组菜单**(一级带多个二级):`SeedMenus` 返回一个 group node(`Path: ""`)+ `Children` 多个叶子(各自 `Path: "/p/<name>/<sub>"`、`Component: "<name>/<Sub>"`);参考 shop 示例(商城分组 → 商品/分类)。validate 允许叶子 path 为 `/p/<name>` 或 `/p/<name>/...`。
- 侧边栏(`route.tsx`):`/p/` 前缀菜单走 `<Link to="/p/$" params={{_splat: menu.path.slice(3)}}>`,非插件菜单保持 `<Link to={menu.path}>`。
- i18n(**自包含,glob 自动收集**):菜单 Title / 页面文案用 key `plg.<name>.<k>`,翻译放插件目录内 `web/src/plugin-components/<name>/i18n.ts`(**default export** `{ en:{...}, zh:{...} }`,en/zh key 同步)。`web/src/lib/i18n.tsx` 用 `import.meta.glob('/src/plugin-components/**/i18n.ts', { eager:true })` 编译期收集,按目录名合并进 `plg.<name>` 命名空间——**与组件 glob 同构**。故 ZIP 安装的插件翻译随 `web/` 解压落回原位、重建即生效,**无需改 `i18n.tsx`、无需改 installer**(手动新增插件同理,生成器已产出 `i18n.ts` 模板)。图标在 `web/src/lib/menu-icons.ts` 注册(未注册回退 CircleIcon)。

## 插件管理页(运行时启用/禁用)
- 后台「插件管理」页 `web/src/routes/_authed/plugins.tsx`(菜单 `/plugins`,Casbin 仅 admin)经 `PluginService`(`proto/zerx/v1/plugin.proto`)透出已编译插件清单 + 运行时启停。
- **编译期约束**:install/uninstall 需重建二进制,UI 不做(仅文档化指引)。enable/disable 是**运行时开关**,无需重建。
- **运行时状态** `internal/plugin/state.go`(`plugin.State`,核心表 `plugin_states`,默认 enabled;`SetEnabled` 用 map upsert 以持久化 `enabled=false`——struct 写会被 `default:true`/零值跳过吞掉)。禁用插件三处 gate:(a) `casbin_interceptor` 在所有放行路径(含 admin)之前 `procEnabled` 拒绝 → `CodeUnavailable`;(b) `MenuService.GetUserMenus` 过滤 `PluginNameOfMenu` 命中的被禁菜单;(c) `scheduler.wrap` 在 fire time 经 `SetHandlerEnabled` 跳过被禁插件 job。三者由 `main.go` 注入同一 `plugin.State`(多副本驱动 postgres/mysql 经 `StartReloader` 周期重载,使一副本的启停传播到其他副本)。
- **数据保留**:禁用**只**关闭菜单/接口/job,**不动表、数据、迁移**;启用即恢复对同一批数据的访问(`SetEnabled` 仅写 `plugin_states.enabled`)。
- **菜单 reconcile**:`syncMenus` 对插件菜单(`plg_` 前缀)**同步** path/component/parent/title/icon/sort/hidden(插件 SeedMenus 是真相);核心菜单仍 insert-only(admin 可编辑)。改插件菜单结构(如扁平页变分组子页)重启即生效,无需手动清库。

## 上传安装 / 卸载(GVA 式,脚手架机制)

> **零插件不变量(关键)**:`internal/plugins/all.go` 含 `var _ = plugin.All` sentinel —— 即使卸载最后一个插件、`Register()` 体清空,`plugin` 包 import 也不会变成 "imported and not used"。**卸载任何插件(含最后一个)都不会让程序 build/run 失败。** installer 写 all.go 前用 `go/parser` 校验仍是合法 Go,拒绝写入坏文件。删 sentinel 会破坏此不变量。
编译期插件无法免重建热插拔(Go 限制)。「上传 zip 安装」= 把源码解压进仓库 + 改 all.go,**装后重启生效**(dev 由 air 自动重编重启;生产手动 `task gen && task build` 重新部署)。
- **打包**:`task plugin-pack -- <name>` → `<name>.zip`(`plugin.json` + `proto/<name>.proto` + `impl/` + `web/`)。
- **安装**:插件管理页「上传插件」→ `PluginService.InstallPlugin(bytes package)` → `internal/plugin/installer` 校验(manifest name 正则+非 Go 关键字、zip-slip 防护、≤256 文件 / ≤25MB、不覆盖已存在)→ 写盘 + all.go 插行(锚点在**独立注释行**,故卸载锚点插件也安全)→ dev 自动 `task gen`。
- **卸载后的中间态(重要)**:卸载只删磁盘源码+all.go 行,但**当前运行的二进制仍编进了该插件**,故 `ListPlugins`(返回 `plugin.All()`)重启前仍列出它。后端检测 impl 目录已不在磁盘 → 置 `PluginInfo.pending_removal=true`,前端该行显「待重启移除」徽章并禁用启停/卸载按钮。dev 下 air 重编重启后 shop 从 registry 消失、该行自动不再出现;生产需 `task build` 重部。
- **卸载**:每行垃圾桶按钮 → `UninstallPlugin(name, purge_data)`。删源码 + all.go 行。`purge_data=true`(确认框点「确定」)走 `teardownData`:**DROP 插件表** + 硬删(`Unscoped()`)menus/grants/jobs/apis/casbin_rule/migrations/plugin_states——真正清空,同名重装从零开始;`purge_data=false` 保留数据。
- **安全门控**:两接口 Casbin 仅 admin **且** `config.UploadAllowed()`(`PLUGIN_UPLOAD_ENABLED` 显式优先,否则仅非 prod)——上传源码 = RCE 级,生产默认关闭。`ListPluginsResponse.upload_allowed` 透给前端,关闭时隐藏上传/卸载按钮。请求体经 `WithReadMaxBytes(25MB)` 限流(connect 在 gate 前缓冲)。
- 注:已编译进进程的插件清单(`ListPlugins`)在重启前不反映新装/已卸载插件——`pending_restart` 提示用户重启。

## 纯前台 / 公开页(匿名,官网类)
插件可服务**无需登录**的前台页(如官网、落地页),与后台菜单/API 并存:
- Plugin 接口 `PublicPages() []PublicPage{Path,Component,Title}` 声明,Path 必须 `/pub/<name>` 或 `/pub/<name>/...`(validate 校验)。
- 匿名接口 `PluginService.ListPublicPages`(在 server.go 的 `public` map,免认证)返回**已启用**插件的公开页;前端公开 splat 路由 `web/src/routes/pub.$.tsx`(**顶层,不在 `_authed` 下,无 auth**)据此按 `/pub/...` 匹配 → glob 加载组件。
- 禁用插件 → `ListPublicPages` 不再返回其页 → `/pub/<name>` 渲染 NotFound(公开页随插件启停)。
- 组合能力:一个插件可同时有 后台菜单(`SeedMenus`)+ 鉴权 API(默认 Casbin)+ 匿名 API(`PublicProcedures`)+ 匿名前台页(`PublicPages`)。示例见 shop 的 `/pub/shop` Landing。

## 生命周期:启用 / 禁用 / 卸载
- **启用**:见上「加一个插件」。迁移/菜单/job 自动接入。
- **禁用(关键约束)**:`assertServicesRegistered` 对**已编译进 proto 描述符**的 zerx.v1 service 强制要求挂载。**仅删 all.go 的 Register 行而保留插件 proto/生成代码 → 启动 fatal**。正确禁用 = 删 `proto/zerx/v1/<name>.proto` → `task gen`(清除已入库 *.pb.go/connect)→ 删 all.go 两行 → 删 impl 目录 → 重建。
- **菜单自动清理**:`syncMenus` 启动时**自动 prune** 孤儿插件菜单——DB 里 `plg_` 前缀但已不属任何已注册插件的菜单(及其 RoleMenu/MenuButton/RoleButton)会被删除。故移除插件 + 重建后,其后台菜单(含分组头)**自动消失**,无需手动清。数据表/apis 不在此自动删(避免误删业务数据)。
- **卸载残留(v1 无自动 teardown)**:syncMenus/syncAPIs 均 insert-only、gormigrate 无 down,卸载后遗留残留;菜单已由启动自动 prune(见上),`<name>_teardown.sql` 覆盖其余(按序):(1) `plg_<name>_*` 数据表;(2)〔菜单已自动清理,teardown 仍含相应 DELETE 作离线清库用〕;(3) `apis` 行;(4) 管理员手配的 `casbin_rule`;(5) `scheduled_jobs` + `job_executions`(handler 前缀 `<name>_`);(6) **`migrations` 账本行 `plg_<name>_*`**——不删则同名重装会跳过建表导致缺表;(7) **`plugin_states` 行**——不删则同名重装可能以禁用态启动。LIKE 下划线均 `ESCAPE '\\'`,防前缀重叠(shop vs shopping)。最小化保留数据则改置菜单 `hidden=true`、插件 disable。

## 可靠性
插件 job panic 已被 `internal/jobs/scheduler.go` `wrap()` 的 `defer recover()` 隔离:单 job panic 记 JobExecution status=error,不拖垮调度器/进程。

## 循环引用约束(改签名时务必守)
两道防线根除 `database→plugin→jobs` 环:(1) Plugin 接口不出现 `jobs.*` 类型(用 `plugin.JobHandler`);(2) `database` 包不 import `plugin`——`Migrate(db, extra)` / `Seed(db, extraMenus []MenuSeed)` 取普通 slice,由 `cmd/server/main.go`(可同时 import 两者)组装入参。job 收集落 main.go(调度器 registry)+ server.go(UI registry)。破坏任一边会让 `go test ./internal/jobs` 编译失败。

## 源码锚点
`internal/plugin/{plugin,registry,validate}.go`、`internal/plugins/all.go`、`internal/plugin/impl/shop/`(示例)、`cmd/zkit/`(脚手架 + 插件生成器:zkit plugin new)、`web/src/routes/_authed/p.$.tsx`(后台 splat)、`web/src/routes/pub.$.tsx`(公开 splat)、`web/src/plugin-components/`、`internal/database/{migrate,seed}.go`、`internal/server/server.go`、`internal/jobs/scheduler.go`。设计全文:`.plans/plugin-system-plan.md`。
