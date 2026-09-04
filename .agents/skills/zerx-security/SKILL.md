---
name: zerx-security
description: "ZKit 认证与安全机制(JWT/会话/刷新/验证码/防爆破/审计日志/文件上传/对象存储)。当处理登录注册、token 刷新、会话管理、登录限流、操作日志、上传、存储 driver 时使用。Keywords: JWT, access token, refresh token, 会话, session, jti, 单点登录, AUTH_SINGLE_SESSION, 验证码, captcha, 防爆破, ratelimit, 锁定, 审计, 操作日志, OperationLog, login_logs, 上传, upload, 对象存储, storage, local, s3, minio, password policy, password reset, SMTP, mailer, TOTP, 2FA, MFA, export, import, xlsx, OpenAPI, docs, cron, job, scheduler, readyz, 安全, security, 认证, auth"
---
# ZKit 认证与安全机制

> Claude 已熟悉 JWT / RBAC / 限流概念;以下是 ZKit 特有规则。授权裁决见 `skill://zerx-authz`。

## JWT 与启动
- **生产必设 `JWT_SECRET`**:缺失则启动失败(`os.Exit(1)`);`APP_ENV=prod` 时 `config.Validate` 还拒绝 `.env.example` 占位值与 <32 字节密钥(dev 不限,测试可用 `"test"`)。
- **无默认账号**:不 seed 管理员;`/register`(public)首个注册者为 `admin`。公开注册默认开启(`site.register_enabled` 缺省/空=开,仅 `"false"` 关闭);关闭后用户数>0 再注册 → `CodeFailedPrecondition`。新用户角色取 `site.register_default_role`(缺省 `user`,禁 `admin`)。邮箱唯一(冲突 → `CodeAlreadyExists`)。开关入口:`SiteSettingsService.UpdateSiteSettings`(后台「网站设置」)。
- token 时效:access **15m**、refresh **168h**。`Claims{ UserID uint64, Roles []string, TokenType string }`(多角色);`Issuer.IssueAccess(userID uint64, roles []string)`、`IssueRefresh / ParseAccess / ParseRefresh`(`internal/auth/jwt.go`)。
- h2c(明文 HTTP/2)仅供 `grpcurl` 等工具;浏览器 SPA 走 HTTP/1.1,不依赖 h2c;`SERVER_H2C_ENABLED=false` 可关(端口裸露无 TLS 终止时)。
- `http.Server`:`ReadHeaderTimeout 10s` 固定,`IdleTimeout`/`WriteTimeout`/优雅关闭取 `SERVER_IDLE_TIMEOUT`(2m)/`SERVER_WRITE_TIMEOUT`(5m,须覆盖上传与导出)/`SERVER_SHUTDOWN_TIMEOUT`(15s)。RPC 请求体上限 `SERVER_MAX_REQUEST_BYTES`(4 MiB,`connect.WithReadMaxBytes`),插件上传单独 25 MiB。
- 子命令:`server healthcheck`(GET 自身 `/readyz`,exit 0/1,供 Dockerfile/compose `HEALTHCHECK`)、`server version`。
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
- **防重放**(`internal/service/totp.go`):`matchTOTPStep` 在 ±1 step 内找出匹配的 time-step(unix/30),`consumeTOTP` 以 `UPDATE user_totps SET last_used_step=? WHERE user_id=? AND last_used_step<?` 原子推进;`RowsAffected==0` 即重放/并发落败 → 拒绝。Login / ActivateTotp / DisableTotp 一律走 `s.consumeTOTP`,**勿直接调 `totp.Validate`**。

## 全局限流(`internal/ratelimit` + `NewRateLimitInterceptor`)
- per-IP token-bucket(`NewLimiter(RPS, Burst, TTL)`);超限返 `CodeResourceExhausted`。IP 取 `clientip.Of(ctx, req)`(见下「客户端 IP」)。
- 拦截器**置于 OperationLog 外层**:被拒请求不落库(避免高压 DB 放大)。`RateLimitConfig{ Enabled(默认 true), RPS=20, Burst=40, TTL=10m }`。

## 验证码 / 登录防爆破(`internal/captcha`、`AuthService` 内 LoginGuard)
- 按 `email|IP` 滑动窗口计数:达 `AUTH_CAPTCHA_THRESHOLD` 后登录需 base64 验证码;达 `AUTH_LOCK_THRESHOLD` 后临时锁定 `AUTH_LOCK_FOR`。
- 登录成功 / 失败均写 `login_logs`。
- 验证码与登录失败计数均落 DB(`captcha_codes` / `login_attempts`,`captcha.New(db)` / `ratelimit.New(..., db)`),跨副本一致;LoginGuard key = `email|clientIP`。

## 审计 / 错误日志(`internal/server/audit_interceptor.go`)
- `OperationLog` 的**唯一写入者**:记录所有写操作与失败请求,**永不记录 body**。写操作判定=`mutatingPrefixes`:`Create/Update/Delete/Set/Sync/Clean/Revoke/Logout/Reorder`。
- 兼 panic 兜底(具名返回 + recover,替代 `connect.WithRecover`,把 panic 与栈写入同一行)。
- 落库经 `opLogWriter`(`newOpLogWriter(db, logger)`,`NewOperationLogInterceptor(w)`):有界队列 `opLogQueueSize=1024` + 单 worker;满则丢弃并 `logger.Error`(`dropped_total` 计数),写失败也记日志;`server.Server.Close(ctx)` 在 `http.Server.Shutdown` 后排空。请求路径永不阻塞。
- 拦截器最外层 `NewErrorSanitizerInterceptor`:客户端只见 `internal error`,操作日志 `Error` 列与 slog(`rpc failed` 行含 `err`、`request_id`)保留原始信息。
- handler 可经 `audit.Record(ctx, detail)`(或 `audit.WithHolder` 取 holder)写 `OperationLog.Detail` 字段;拦截器落库时读 `holder.Detail`。
- 错误日志 = OperationLog 中 `status != "ok"` 的行,**无独立表**;LoginLog / ListOperationLogs / ListErrorLogs 支持 status/method/start_at/end_at 过滤。

## 上传与对象存储(`internal/storage`、`internal/media`)
- `/api/upload`(multipart):任意已登录用户可用;**20MB** 上限、扩展名白名单 **+ 内容嗅探**(`sniffAllowed`:前 1 KiB `http.DetectContentType` 须落在 `allowedContentTypes[ext]`;`.svg` 另须含 `<svg` 且不含 `<script`;不匹配 400 `unsupported file content`)、uuid key 防碰撞;表单 `visibility` 字段(`public|authenticated|private`,非法值兜底 `private`)写入 `File.Visibility`。返回 `url` = `media.ResolveFile(key, visibility)`。
- 前端用 `transport.ts` 的 `authedFetch`(共享 401 刷新)。
- driver = `local`(磁盘)| `s3`(minio-go);`StorageConfig{Driver, LocalDir, LocalBaseURL, S3Endpoint, S3AccessKey, S3SecretKey, S3Bucket, S3Region, S3Secure, S3PublicURL, SignedURLTTL}`(`STORAGE_SIGNED_URL_TTL` 默认 `1h`;配置见 `.env.example`)。
- `Storage` 接口(`storage.go`):`Save` **不返回 url**(只 error);`PublicURL(key) string`(无 public base URL 时返 "")、`Open(ctx,key) (io.ReadSeekCloser, time.Time, error)`、`Presign(ctx,key,ttl) (string, error)`(local 返 "",nil)。URL 不再落库——`File` 存 `Key`+`Visibility`,展示时经 media 动态解析。

### 文件可见性与媒体访问(`internal/media` + `internal/server/media.go`)
- **三层可见性**(`model.Visibility{Public,Authenticated,Private}`,默认 `private`),区别于 Casbin **接口**鉴权——这是 **blob 内容**鉴权:`public` 任何人;`authenticated` 任意登录用户;`private` 仅 owner(`UploadedBy`)或 admin。`FileService.DeleteFile` 同样限 owner/admin(否则 `CodePermissionDenied`)。
- **签名 URL = capability token**:local 用 HMAC-SHA256(`sign(key,exp)` over `key\nexp`,base64url),签名密钥 `sha256(JWT.Secret + "/media-url-v1")`(`server.New` 构造,仅 local);s3 用原生预签名 GET(`store.Presign`)。`Verify` 常量时间比对 + 校验 `exp`。TTL:文件 `SignedURLTTL`、头像固定 `avatarTTL=24h`。
- `media.Media` 解析:`ResolveFile(key,vis)`(public→`PublicURL`,否则签名 URL)、`ResolveAvatar(stored)`(空/外链直通,否则签名 `avatarTTL`)、`ResolveLogo(stored)`(logo 恒 public)、`NormalizeStored(value)`(写侧把签名/旧 URL 归一为裸 key,幂等)、`Open(ctx,key)`。
- `server.go`:driver=local 时 `LocalBaseURL+"/"` 挂 **`mediaHandler`**(可见性鉴权,非裸 `http.FileServer`):`public` 直发(可缓存);protected 需有效签名(capability)**或** Bearer access token 满足可见性。响应恒带 `nosniff` + `CSP: sandbox`;仅 `inlineTypes`(png/jpeg/gif/webp/pdf/mp4/mp3 且与扩展名一致)内联,其余(含 svg)`Content-Disposition: attachment`。`mediaResolver` 注入 `AuthService`/`UserService`/`FileService`/`uploadHandler`。

## 导出 / 导入 / API 文档(非 connectRPC,裸 HTTP)
- `/api/export/{users,operation-logs,login-logs,error-logs}`(xlsx)、`/api/import/users`、`/api/import/users/template`:**手写 JWT 认证 + Casbin 鉴权**(对应 List procedure,`exportHandler(issuer, enforcer, db)` 等),不走拦截器链。
- `DOCS_ENABLED=true`(默认)挂 `/api/openapi.yaml`(go:embed)+ `/api/docs`(Scalar);`gen/openapi/` 由 `task gen` 产出。
- `/readyz`:DB ping(2s 超时)就绪探针;`/healthz`:存活探针(恒 200)。

## 定时任务(`internal/jobs`)
- `jobs.New(db, registry, logger)` + `Start()` 调度 enabled 任务;`main.go` 装配并 `defer Shutdown()`。`NewRegistry(db)` 绑内置 handler(如 `log_cleanup`);`ValidCron` 校验 5 段标准 cron。`JobService` 管 `scheduled_jobs` / `job_executions`、`RunNow`。

## 配置(`internal/config/config.go`)
- `Config{ Server, DB, JWT, Auth, Storage, Password, SMTP, RateLimit, Plugin, Log, Env }`。
- `AuthConfig{ SingleSession, CaptchaThreshold, LockThreshold, LockFor }`;`ServerConfig{ Addr, DocsEnabled, TrustedProxies, H2C, IdleTimeout, WriteTimeout, ShutdownTimeout, MaxRequestBytes, CORSOrigins, SecurityHeaders, CSP }`;`DBConfig{ Driver, DSN, MaxOpenConns, MaxIdleConns, ConnMaxLifetime, ConnMaxIdleTime }`;`LogConfig{ Level, Format }`(`cfg.LogLevel()` / `cfg.LogJSON()` 按 Env 缺省);`PasswordPolicyConfig` / `SMTPConfig` / `RateLimitConfig` 见上各节,全量 env 见 `.env.example`。
- `Load()`:非 prod 先 `godotenv.Load()`,再 `env.ParseAs[Config]()`,最后 `cfg.Validate()`(prod 密钥强度、LOG_* 合法性、池/请求体上限 >0)。

## HTTP 中间件(`internal/server/middleware.go`)
- 顺序(外→内):`clientip.Middleware → withRequestID → withSecurityHeaders(若 SECURITY_HEADERS_ENABLED) → withCORS(若 CORS_ALLOWED_ORIGINS) → mux`。
- request id:入站 `X-Request-Id`(≤64 字符 `[A-Za-z0-9._-]`)沿用,否则 uuid;总是回显;`requestIDFrom(ctx)` 进 RPC 日志。
- 安全头:`defaultCSP`(`script-src 'self'`、`style-src 'self' 'unsafe-inline'`、`img-src 'self' data: blob: https: http:`、`frame-ancestors 'none'`)、`X-Frame-Options: DENY`、nosniff、Referrer-Policy、Permissions-Policy;`r.TLS != nil` 或信任代理报 `X-Forwarded-Proto: https` 时加 HSTS。`SERVER_CSP` 整体覆盖;`/api/docs` 自行放宽以加载 jsDelivr。**前端不得引入 inline script**。
- CORS:`rs/cors` + `connectrpc.com/cors` 头清单,仅前后端分域时配置。

## 客户端 IP(`internal/clientip`)
- `NewResolver(cfg.Server.TrustedProxies)`(CIDR/IP 列表,空=从不信任代理头);`Middleware` 每请求解析一次入 ctx;`clientip.Of(ctx, req)` 取值(ctx 缺失时回退 `req.Peer().Addr` 去端口,便于直接调 handler 的测试)。
- 规则:对端在信任列表内才读 `X-Forwarded-For`(从右向左取第一个非信任 hop,全信任则取最左)→ `X-Real-Ip` → 对端;IPv4-mapped IPv6 归一。限流、LoginGuard、登录日志、操作日志、HSTS 判定共用。**新代码禁止再直接读 `req.Peer().Addr`**。

## 安全模型注意
- **进程内状态**:全局限流令牌桶、`param.Cache`(多副本 30s 重载)、Casbin 决策缓存、操作日志队列为每实例独立;LoginGuard / 验证码 / job 锁已落 DB。无 Redis 是有意取舍。
- **IP 来源**:经 `TRUSTED_PROXIES` 门控的 `clientip`(见上);反代之后**必须**配置,否则所有用户共享代理 IP(限流互相误伤、锁定互相牵连)。
- **纯 Go / CGO-free**:构建 `CGO_ENABLED=0`;新增依赖须纯 Go。

## 源码锚点
`internal/auth/{jwt.go,policy.go}`、`internal/{clientip,ratelimit,captcha,mailer,audit,jobs,storage,media}/`、`internal/server/{middleware.go,audit_interceptor.go,interceptors.go,export.go,import.go,docs.go,httpauth.go,upload.go,media.go,server.go}`、`internal/service/totp.go`、`internal/model/{file.go,totp.go}`、`internal/config/config.go`、`cmd/server/main.go`(healthcheck/version)、`web/src/lib/transport.ts`、`.env.example`。
