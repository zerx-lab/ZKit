---
name: zerx-add-module
description: "在 ZKit 端到端新增一个管理模块的完整清单(proto→model→migrate→service→server→前端页→i18n→seed→routeTree→授权)。当用户要新增一个后台管理模块/资源/CRUD 页面(如商品、订单、配置项)时使用。Keywords: 新增模块, 管理模块, 后台模块, CRUD, 资源, 端到端, 完整流程, scaffold, 新建页面, add module, new module, 菜单, seed, 迁移, 全栈新增"
---
# ZKit 端到端新增管理模块

> 编排清单;各步细节互链对应 skill,不复制其正文。

## 有序清单(9 步)
1. **proto**:`proto/zerx/v1/<mod>.proto` 加 service + messages(方法名避开 JS 保留字;校验注解写字段上)→ `task gen`。细节 → `skill://zerx-backend`。
2. **model**:`internal/model/<mod>.go`,标准字段块(`ID/CreatedAt/UpdatedAt/DeletedAt`)。在 `internal/database/migrate.go` 的 `0001_baseline` AutoMigrate 快照列表(现 21 个模型)对应位置注册 `&model.<Mod>{}`;如需改列/回填另加 `000N_*` 迁移(`HasColumn` 守卫)。
3. **service**:`internal/service/<mod>_service.go`,照抄 `user_service.go` 范式,**无 `RequireRole`**(授权交 Casbin)→ `skill://zerx-authz`、`skill://zerx-backend`;转换函数加进 `convert.go`。
4. **server**:`internal/server/server.go` 用 `reg(zerxv1connect.New<Mod>ServiceHandler(service.New<Mod>Service(...), opts))` 注册;按需把 procedure 加入 `public` / `selfServe`(`assertServicesRegistered` 会校验挂载)。
5. **前端页**:`web/src/routes/_authed/<mod>.tsx`,复刻 `users.tsx`/`params.tsx` 范式;增删改按钮用 `<Can code="<资源>:<动作>">` 包裹 → `skill://zerx-frontend`。
6. **i18n**:`web/src/lib/i18n.tsx` 的 `en` 与 `zh` **同步**加 key(结构锁,缺 key 编译失败)。
7. **seed**:在 `internal/database/seed.go` 的 `seedMenuTree` 加一节点;Title 存 i18n key,Icon 存 lucide 名,并在 `web/src/lib/menu-icons.ts` 的 `iconByName` 注册图标。
8. **routeTree**:`task frontend:build`(或 dev)让 Vite 插件重生成 `web/src/routeTree.gen.ts` 并提交。
9. **授权**:如需授予非 admin,到「角色管理」页为角色分配菜单 / API procedure / 按钮。

## seed 实测细节(`internal/database/seed.go`)
- 节点类型:`seedMenuNode{ node seedMenu, children []seedMenuNode }`;`seedMenu{ menu model.Menu, buttons []model.MenuButton, userVisible bool }`。
- 顶层菜单(`Path` 非空)或分组标题(`Path == ""`,作 group heading,子项放 `children`)。
- 元素例:
  ```go
  {node: seedMenu{menu: model.Menu{Path: "/dashboard", Name: "dashboard", Title: "nav.dashboard", Icon: "LayoutDashboardIcon", Sort: 1}, userVisible: true}}
  ```
- 标准 CRUD 按钮用 helper:`crudButtons("<资源>", "<中文标签>")` → 生成 `<资源>:create/update/delete`;额外按钮直接 `append`。
- **幂等键 = Role 表为空**:首次播种 admin/user 角色、DFS 插菜单树、RoleMenu(admin 全 / user 仅 `userVisible`)、RoleButton(仅 admin)、`syncAPIs()` upsert 全量 procedure。
- 已初始化库:每次启动**增量**reconcile——`syncMenus`(按 `menu.Name` 匹配,**仅插入不改不删**)+ `syncAPIs`(按 procedure upsert,会 prune 过期行)。新菜单重启即生效,无需 DB reset。

## procedure 目录来源
`internal/apispec/apispec.go` 的 `Procedures() []Proc{Procedure, Service, Method}` 遍历 `protoregistry.GlobalFiles` 筛 `zerx.v1.` 前缀;`task gen` 后新 service 的 procedure 自动入目录(`syncAPIs` 落库)。

## 源码锚点
`internal/database/seed.go`、`internal/database/migrate.go`、`internal/apispec/apispec.go`、`web/src/routes/_authed/`(范式页)、`web/vite.config.ts`。
