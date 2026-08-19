---
name: zerx-security
description: "ZKit 认证与安全机制(JWT/会话/刷新/验证码/防爆破/审计日志/文件上传/对象存储)。当处理登录注册、token 刷新、会话管理、登录限流、操作日志、上传、存储 driver 时使用。Keywords: JWT, access token, refresh token, 会话, session, jti, 单点登录, AUTH_SINGLE_SESSION, 验证码, captcha, 防爆破, ratelimit, 锁定, 审计, 操作日志, OperationLog, login_logs, 上传, upload, 对象存储, storage, local, s3, minio, password policy, password reset, SMTP, mailer, TOTP, 2FA, MFA, export, import, xlsx, OpenAPI, docs, cron, job, scheduler, readyz, 安全, security, 认证, auth"
---
# ZKit 认证与安全机制

> Claude 已熟悉 JWT / RBAC / 限流概念;以下是 ZKit 特有规则。授权裁决见 `skill://zerx-authz`。

## JWT 与启动
- **生产必设 `JWT_SECRET`**:缺失则启动失败(`os.Exit(1)`)。
- **无默认账号**:不 seed 管理员;`/register`(public)首个注册者为 `admin`。公开注册默认开启(`site.register_enabled` 缺省/空=开,仅 `"false"` 关闭);关闭后用户数>0 再注册 → `CodeFailedPrecondition`。新用户角色取 `site.register_default_role`(缺省 `user`,禁 `admin`)。邮箱唯一(冲突 → `CodeAlreadyExists`)。开关入口:`SiteSettingsService.UpdateSiteSettings`(后台「网站设置」)。
- token 时效:access **15m**、refresh **168h**。`Claims{ UserID uint64, Roles []string, TokenType string }`(多角色);`Issuer.IssueAccess(userID uint64, roles []string)`、`IssueRefresh / ParseAccess / ParseRefresh`(`internal/auth/jwt.go`)。
- h2c(明文 HTTP/2)仅供 `grpcurl` 等工具;浏览器 SPA 走 HTTP/1.1,不依赖 h2c。
- 前端刷新:`transport.ts` 实现 401 → single-flight 刷新 → 重试一次 → 仍失败清 token 跳登录。

## 会话(多点登录)
- refresh 的 **jti = 会话 ID**,对应 `user_sessions` 行。
- `AUTH_SINGLE_SESSION=true`:每次登录在同一事务内删除该用户其它会话(单端);默认允许多端,「会话管理」页查看/下线。
- 撤销会话**即时阻断 refresh**;access 无状态,自然过期后(≤15m)彻底失效。即时强制下线需每请求查会话(默认不开)。

## 密码策略(`internal/auth/policy.go`)
- `NewPolicy(cfg.Password) *Policy`;`Validate(pw)`(长度 + 大小写/数字/符号开关)、`CheckHistory(ctx, db, userID, newPlain)`(拒绝最近 N 个旧密码)、`RecordHistory(ctx, db, userID, hash)`(写 `password_history`,超 N 裁剪)。
- 配置 `PasswordPolicyConfig{ MinLength, RequireUpper/Lower/Digit/Symbol, HistoryCount }`(默认 8 位、仅要求数字、保留 3 条历史)。
- 用户改密 / 重置 / 管理员建用户均经 `policy`(`AuthService.ChangePassword`、`UserService`)。

## 密码重置(邮件,`internal/mailer`)
- public RPC `AuthService.RequestPasswordReset` → 生成 `password_reset_tokens` 行,经 `mailer.Send(ctx, to, subject, htmlBody)` 发链接;`ConfirmPasswordReset` 校验 token + `policy` 设新密码。
- `NewMailer(cfg.SMTP, logger)`;`SMTP_ENABLED=false` 时邮件仅记日志(开发不阻塞流程)。

## TOTP 二次验证(2FA)
- selfServe RPC `SetupTotp`(出 secret/二维码)→ `ActivateTotp`(验码启用,落 `user_totp` + `totp_recovery_codes`,`User.TotpEnabled=true`)→ `DisableTotp`;管理员 `UserService.DisableUserTotp`。
- `Login` 入参加 `totp_code`;启用 2FA 的用户首轮无码登录返回 `LoginResponse.totp_required=true`。

## 全局限流(`internal/ratelimit` + `NewRateLimitInterceptor`)
- per-IP token-bucket(`NewLimiter(RPS, Burst, TTL)`);超限返 `CodeResourceExhausted`。
- 拦截器**置于 OperationLog 外层**:被拒请求不落库(避免高压 DB 放大)。`RateLimitConfig{ Enabled(默认 true), RPS=20, Burst=40, TTL=10m }`。

## 验证码 / 登录防爆破(`internal/captcha`、`AuthService` 内 LoginGuard)
- 按 `email|IP` 滑动窗口计数:达 `AUTH_CAPTCHA_THRESHOLD` 后登录需 base64 验证码;达 `AUTH_LOCK_THRESHOLD` 后临时锁定 `AUTH_LOCK_FOR`。
- 登录成功 / 失败均写 `login_logs`。
- 验证码内存 store(进程内)。

## 审计 / 错误日志(`internal/server/audit_interceptor.go`)
- `OperationLog` 的**唯一写入者**:记录所有写操作与失败请求,**永不记录 body**。写操作判定=`mutatingPrefixes`:`Create/Update/Delete/Set/Sync/Clean/Revoke/Logout/Reorder`。
- 兼 panic 兜底(具名返回 + recover,替代 `connect.WithRecover`,把 panic 与栈写入同一行)。
- handler 可经 `audit.Record(ctx, detail)`(或 `audit.WithHolder` 取 holder)写 `OperationLog.Detail` 字段;拦截器落库时读 `holder.Detail`。
- 错误日志 = OperationLog 中 `status != "ok"` 的行,**无独立表**;LoginLog / ListOperationLogs / ListErrorLogs 支持 status/method/start_at/end_at 过滤。

## 上传与对象存储(`internal/storage`、`internal/media`)
- `/api/upload`(multipart):任意已登录用户可用;**20MB** 上限、扩展名白名单、uuid key 防碰撞;表单 `visibility` 字段(`public|authenticated|private`,非法值兜底 `private`)写入 `File.Visibility`。返回 `url` = `media.ResolveFile(key, visibility)`。
- 前端用 `transport.ts` 的 `authedFetch`(共享 401 刷新)。
- driver = `local`(磁盘)| `s3`(minio-go);`StorageConfig{Driver, LocalDir, LocalBaseURL, S3Endpoint, S3AccessKey, S3SecretKey, S3Bucket, S3Region, S3Secure, S3PublicURL, SignedURLTTL}`(`STORAGE_SIGNED_URL_TTL` 默认 `1h`;配置见 `.env.example`)。
- `Storage` 接口(`storage.go`):`Save` **不返回 url**(只 error);`PublicURL(key) string`(无 public base URL 时返 "")、`Open(ctx,key) (io.ReadSeekCloser, time.Time, error)`、`Presign(ctx,key,ttl) (string, error)`(local 返 "",nil)。URL 不再落库——`File` 存 `Key`+`Visibility`,展示时经 media 动态解析。

### 文件可见性与媒体访问(`internal/media` + `internal/server/media.go`)
- **三层可见性**(`model.Visibility{Public,Authenticated,Private}`,默认 `private`),区别于 Casbin **接口**鉴权——这是 **blob 内容**鉴权:`public` 任何人;`authenticated` 任意登录用户;`private` 仅 owner(`UploadedBy`)或 admin。`FileService.DeleteFile` 同样限 owner/admin(否则 `CodePermissionDenied`)。
- **签名 URL = capability token**:local 用 HMAC-SHA256(`sign(key,exp)` over `key\nexp`,base64url),签名密钥 `sha256(JWT.Secret + "/media-url-v1")`(`server.New` 构造,仅 local);s3 用原生预签名 GET(`store.Presign`)。`Verify` 常量时间比对 + 校验 `exp`。TTL:文件 `SignedURLTTL`、头像固定 `avatarTTL=24h`。
- `media.Media` 解析:`ResolveFile(key,vis)`(public→`PublicURL`,否则签名 URL)、`ResolveAvatar(stored)`(空/外链直通,否则签名 `avatarTTL`)、`ResolveLogo(stored)`(logo 恒 public)、`NormalizeStored(value)`(写侧把签名/旧 URL 归一为裸 key,幂等)、`Open(ctx,key)`。
- `server.go`:driver=local 时 `LocalBaseURL+"/"` 挂 **`mediaHandler`**(可见性鉴权,非裸 `http.FileServer`):`public` 直发(可缓存);protected 需有效签名(capability)**或** Bearer access token 满足可见性。`mediaResolver` 注入 `AuthService`/`UserService`/`FileService`/`uploadHandler`。

## 导出 / 导入 / API 文档(非 connectRPC,裸 HTTP)
- `/api/export/{users,operation-logs,login-logs,error-logs}`(xlsx)、`/api/import/users`、`/api/import/users/template`:**手写 JWT 认证 + Casbin 鉴权**(对应 List procedure,`exportHandler(issuer, enforcer, db)` 等),不走拦截器链。
- `DOCS_ENABLED=true`(默认)挂 `/api/openapi.yaml`(go:embed)+ `/api/docs`(Scalar);`gen/openapi/` 由 `task gen` 产出。
- `/readyz`:DB ping(2s 超时)就绪探针;`/healthz`:存活探针(恒 200)。

## 定时任务(`internal/jobs`)
- `jobs.New(db, registry, logger)` + `Start()` 调度 enabled 任务;`main.go` 装配并 `defer Shutdown()`。`NewRegistry(db)` 绑内置 handler(如 `log_cleanup`);`ValidCron` 校验 5 段标准 cron。`JobService` 管 `scheduled_jobs` / `job_executions`、`RunNow`。

## 配置(`internal/config/config.go`)
- `Config{ Server, DB, JWT, Auth, Storage, Password, SMTP, RateLimit, Env }`。
- `AuthConfig{ SingleSession, CaptchaThreshold, LockThreshold, LockFor }`;`ServerConfig{ Addr, DocsEnabled }`;新增 `PasswordPolicyConfig` / `SMTPConfig` / `RateLimitConfig`(见上各节,全量 env 见 `.env.example`)。
- `Load()`:非 prod 先 `godotenv.Load()`,再 `env.ParseAs[Config]()`。

## 安全模型注意
- **进程内状态(单实例假设)**:LoginGuard、`param.Cache`、captcha store、Casbin 决策缓存均进程内,水平扩容不跨实例一致(本脚手架无 Redis,是有意取舍)。
- **IP 来源**:取 `req.Peer().Addr` 去端口;反向代理后会变代理 IP,生产经代理需自行接 `X-Forwarded-For`。
- **纯 Go / CGO-free**:构建 `CGO_ENABLED=0`;新增依赖须纯 Go。

## 源码锚点
`internal/auth/{jwt.go,policy.go}`、`internal/{ratelimit,captcha,mailer,audit,jobs,storage,media}/`、`internal/server/{audit_interceptor.go,interceptors.go,export.go,import.go,docs.go,httpauth.go,upload.go,media.go,server.go}`、`internal/model/file.go`、`internal/config/config.go`、`web/src/lib/transport.ts`、`.env.example`。
