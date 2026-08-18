# Watchdog notes(本项目架构红线,advisor 语义级 review 重点盯)

> 这些是正则/ast 抓不住、但违反即出 bug 或破坏安全模型的红线。TTSR 正则规则见 `.omp/rules/`,互补不重复。

## 鉴权与角色(安全边界)
- service handler 的内联角色判断,**三类法**区分(只对第一类报违规):
  - **(a) 违规——应进 Casbin**:角色判断决定**整个 procedure** 的放行/拒绝、与具体资源无关(可表达为「角色 × procedure」),如 handler 顶部 `if !slices.Contains(claims.Roles, model.RoleAdmin) { return PermissionDenied }`。这类必须删除,授权交 Casbin 拦截器(`sub=角色 code`、`obj=procedure`)。
  - **(b) 合法——resource ownership / self-serve**:角色判断**叠加资源归属**(owner / UserID / 行谓词),Casbin 的「角色 × procedure」模型结构上无法表达行归属,**必须留在 handler**。范式:`auth_service.go:345/376`(`target != claims.UserID && !slices.Contains(claims.Roles, model.RoleAdmin)`)、`file_service.go:79`(`!...RoleAdmin) && f.UploadedBy != claims.UserID`)。
  - **(c) 合法——admin 捷径 + 按角色 scope 数据**:admin 全量、非 admin 按角色过滤数据行,如 `file_service.go:43`、`menu_service.go:187/232` 的 `if slices.Contains(claims.Roles, model.RoleAdmin)` 分支。属数据可见性过滤,非接口鉴权,留 handler。
  - 真实形态是 `claims.Roles []string` + `slices.Contains`(不是单数 `claims.Role`);`RequireRole` 字面量在 handler 内一律违规(双源裁决漂移)。
- 把 RBAC 管理类 procedure(RoleService/MenuService/ApiService 的写、SetRolePermissions、SyncApis)授予非 admin 角色 = **变相提权**,需明确意图。
- 角色 `code` 改名 = 破坏事实主键(`User.Roles` / `user_roles` 关联 / casbin sub / RoleMenu/RoleButton 关联)。内置 `admin`/`user` code 不可改。
- 按钮权限(`<Can code>`)纯前端 UX 显隐,**非安全边界**;真正鉴权在同名 procedure 的 Casbin 策略。

## 进程内缓存一致性(隐蔽红线)
- 写 SysParam **必须经 `param.Cache.Set(ctx, k, v)`**(自动同步缓存),或写后紧跟 `cache.Reload(ctx)`;**绝不可裸 `gorm.G[SysParam]...Create/Updates` 而不刷新缓存** → 内存与 DB 不一致(stale 参数)。
- 同理排查任何"进程内缓存 + DB"成对状态(param/casbin 决策缓存等)写后是否刷新。

## 数据访问
- 自定义 querier(`internal/model/querier.go`)raw SQL:**必须 `@name` 绑定参数 + 显式 `AND deleted_at IS NULL`**(raw SQL 绕过 GORM 软删)。
- 新增模型须加进 `internal/database/migrate.go` 的 gormigrate migration(`0001_baseline` 的 `AutoMigrate(...)` 列表),不是裸 AutoMigrate。
- 读 proto 字段用 getter(`req.Msg.GetXxx()`),直接取字段会被 nilaway 标记。

## 定时任务
- 新增 job handler 必须注册进 `internal/jobs/registry.go` 的 `NewRegistry`(UI 只能调度白名单内的);cron 用 5 段标准格式(`ValidCron` 校验,无秒位)。

## 日志与审计
- OperationLog 唯一写入者是 `internal/server/audit_interceptor.go`,**永不记录 body**;新增写操作前缀须被 `mutatingPrefixes` 覆盖才会记录,否则审计漏记。
- 错误日志 = OperationLog 中 `status != "ok"` 的行,无独立表;别另建错误日志表/写入点。

## 前端
- 新增 i18n key 时 en/zh 是否同步;`t(key)` 的 key 是否真实存在于字典。
- 误用 react-hook-form / shadcn `form` 组件而非 `@tanstack/react-form`;Zod 须用 v4 顶层格式函数(`z.email()`)。
- 页级功能按钮须不透明白底(`outline`=`bg-card`);`ghost` 仅顶栏 icon / 表格行内。禁止 `Button` 覆写 `bg-transparent`/`bg-background`。

## 构建
- 新增依赖必须纯 Go(`CGO_ENABLED=0`),不得引入 cgo 依赖。
