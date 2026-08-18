---
name: zerx-authz
description: "ZKit 接口授权与访问控制。当新增/修改 connectRPC handler、注册 service、配置 public/selfServe、调整 Casbin 策略、角色权限、菜单/按钮可见性时使用。Keywords: 授权, 鉴权, 权限, Casbin, RequireRole, public, selfServe, admin 绕过, procedure, RBAC, 角色, role, 三层访问控制, enforcer, SetRolePermissions, RoleMenu, RoleButton, Can, 菜单权限, 按钮权限, authorization, access control"
---
# ZKit 授权与三层访问控制

> Claude 已熟悉 Casbin / RBAC 概念;以下是 ZKit 特有规则。

## 铁律(必读)
- handler **一律不写** `auth.RequireRole(...)`。授权的**唯一权威**是 Casbin 拦截器(`internal/auth/casbin_interceptor.go`):`sub=角色 code`、`obj=connectRPC procedure`,精确匹配。
- `auth.RequireRole(ctx, role)` 存在于 `internal/auth/context.go`,但 **handler 禁用**——它会与拦截器双源裁决造成漂移。

## 内联角色判断:三类法(铁律的精确边界)
「handler 不写授权」指的是**接口级**授权(角色 × procedure 决定整个方法放行/拒绝)。但**资源级**判断必须留 handler——Casbin 模型(sub=角色、obj=procedure)结构上无法表达行归属。区分:
- **(a) 违规,进 Casbin**:handler 顶部 `if !slices.Contains(claims.Roles, model.RoleAdmin) { return PermissionDenied }`、`RequireRole(...)`——与具体资源无关、整方法放行/拒绝。删除,交拦截器。
- **(b) 合法,留 handler(ownership / self-serve)**:角色叠加资源归属。范式 `auth_service.go` ListSessions/RevokeSession:`target != claims.UserID && !slices.Contains(claims.Roles, model.RoleAdmin)`;`file_service.go` DeleteFile:`!...RoleAdmin) && f.UploadedBy != claims.UserID`。
- **(c) 合法,留 handler(admin 捷径 + 按角色 scope 数据)**:admin 全量、非 admin 过滤数据行。范式 `file_service.go` ListFiles 的 `visibility IN ? OR uploaded_by = ?`、`menu_service.go` GetUserMenus/GetUserButtons 的 admin 分支。属数据可见性,非接口鉴权。

## 三层访问控制(顺序固定)
裁决顺序(`casbin_interceptor.go` 决策链):
```
public[proc]                 → 放行(无需 claims)
无 claims                    → CodeUnauthenticated
selfServe[proc]              → 放行(任意已认证调用者)
claims.Roles 任一 ==admin    → 放行(绕过 Casbin)
否则 任一 role Enforce 命中   → 放行;全不命中 → CodePermissionDenied
```
签名:`NewCasbinInterceptor(enforcer *casbin.SyncedCachedEnforcer, public, selfServe map[string]bool)`。
- `obj = req.Spec().Procedure`;`sub` **逐个**取自 `claims.Roles`(多角色,`[]string`);claims 来自 `auth.ClaimsFromContext(ctx)`。任一角色被授予即放行。
- 拦截器链(`server.go`,洋葱,数组靠前=外层先执行):`NewLoggingInterceptor → [NewRateLimitInterceptor(若 cfg.RateLimit.Enabled)] → auth.NewAuthInterceptor(issuer, public) → NewOperationLogInterceptor → auth.NewCasbinInterceptor(enforcer, public, selfServe) → validate.NewInterceptor`。
  - RateLimit **故意置于 OperationLog 外层**:被限流的请求不落库(避免高压下 DB 放大),返回 `CodeResourceExhausted`。
  - OperationLog 拦截器**兼 panic 兜底**,已替代 `connect.WithRecover`(无单独 recover)。

## public(免认证,共 6 项)
```
AuthServiceLoginProcedure
AuthServiceRegisterProcedure
AuthServiceRefreshProcedure
AuthServiceGetCaptchaProcedure
AuthServiceRequestPasswordResetProcedure
AuthServiceConfirmPasswordResetProcedure
```

## selfServe(已登录即放行,共 13 项)
```
AuthServiceMeProcedure
AuthServiceLogoutProcedure
AuthServiceListSessionsProcedure
AuthServiceRevokeSessionProcedure
MenuServiceGetUserMenusProcedure
MenuServiceGetUserButtonsProcedure
DictServiceGetDictByTypeProcedure
SiteSettingsServiceGetSiteSettingsProcedure
AuthServiceChangePasswordProcedure
AuthServiceUpdateProfileProcedure
AuthServiceSetupTotpProcedure
AuthServiceActivateTotpProcedure
AuthServiceDisableTotpProcedure
```

## 策略生效与安全须知
- 把任意 procedure(含写操作)授予某角色 → **即时生效**(SyncedCachedEnforcer 封装在 `internal/casbin/`,gorm-adapter)。这是 RBAC 可用的关键。
- 角色权限写操作:`RoleService.SetRolePermissions`;接口目录同步:`ApiService.SyncApis`。
- **安全须知**:把 RBAC 管理类 procedure(RoleService / MenuService / ApiService 的写、`SetRolePermissions`、`SyncApis`)授予非 admin = 授予提权能力。默认仅 admin 拥有(靠绕过),新角色默认无任何策略。

## 菜单 / 按钮(不走 Casbin)
- 菜单可见性、按钮权限是独立关联表 **RoleMenu / RoleButton**,与 Casbin 无关。
- `<Can code>` 仅前端 UX 显隐,**非安全边界**(详见 `skill://zerx-frontend`)。
- 约定 button code = `<资源>:<动作>`(如 `user:create`);其**真正鉴权**始终在同名 procedure 的 Casbin 策略上。

## Role-as-code 局限
- 角色以字符串 `code` 为业务键,**code 不可改名**(是 casbin sub / `user_roles` 关联表的事实主键)。
- **多角色**:`User.Roles []string`(`model.UserRole` = `user_roles` 关联表),JWT `Claims.Roles []string`;任一角色命中即放行。但 casbin **无 `g` 角色继承**;**仅接口级鉴权,无数据/行级权限**。

## 源码锚点
`internal/server/server.go`(public/selfServe/chain/reg)、`internal/auth/casbin_interceptor.go`、`internal/auth/interceptor.go`、`internal/auth/context.go`(`RequireRole` 用 `slices.Contains(claims.Roles, role)`)、`internal/casbin/`。
