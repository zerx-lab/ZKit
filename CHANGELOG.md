# Changelog

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 SemVer。

## [Unreleased]

### 安全
- 客户端 IP 解析新增 `TRUSTED_PROXIES`（`internal/clientip`）：仅当对端在信任列表内才采信 `X-Forwarded-For` / `X-Real-Ip`；限流、登录防爆破、审计 IP 统一经此解析（此前反代之后全部退化为代理 IP）。
- TOTP 防重放：`user_totps.last_used_step` 记录最近一次接受的 time-step，同 step 的验证码二次提交被拒（登录 / 绑定 / 解绑三处生效，条件更新保证并发只成功一次）。
- 生产环境（`APP_ENV=prod`）拒绝 `.env.example` 占位 `JWT_SECRET` 与短于 32 字节的密钥。
- 全局安全响应头（`SECURITY_HEADERS_ENABLED` / `SERVER_CSP`）：CSP `script-src 'self'`、`X-Frame-Options: DENY`、nosniff、Referrer-Policy、Permissions-Policy；经 TLS（直连或信任代理 `X-Forwarded-Proto=https`）时附 HSTS。`/api/docs` 单独放宽以加载 jsDelivr 上的 Scalar。
- `CodeInternal` / `CodeUnknown` 错误在最外层拦截器统一脱敏为固定文案，原始错误只进服务端日志与操作日志。
- 上传内容嗅探：按扩展名白名单外再校验前 1 KiB 的 `http.DetectContentType`；`.svg` 须含 `<svg` 且不含 `<script`；受保护文件除图片 / PDF / 音视频外一律 `Content-Disposition: attachment`。
- 所有 connectRPC 请求体上限 `SERVER_MAX_REQUEST_BYTES`（默认 4 MiB；插件上传单独 25 MiB）。
- `SERVER_H2C_ENABLED` 可关闭明文 HTTP/2。
- golangci 启用 `gosec` / `bodyclose` / `noctx` / `sqlclosecheck` / `errorlint` / `rowserrcheck`。

### 修复
- 删除用户改为事务：邮箱改写为 `deleted:<id>:<email>` 释放唯一索引（可用同邮箱重新注册），并级联清理角色关联 / 会话 / TOTP / 恢复码 / 密码历史 / 重置令牌；迁移 `0008` 追平历史软删行。
- 删除菜单时存在子菜单返回 `FailedPrecondition`，不再产生孤儿节点。
- `ListUsers` 关键字搜索分支补齐分页与 total。
- `PageRequest` 在 proto 层约束 `page >= 0`、`0 <= page_size <= 100`。
- 插件卸载 `purge_data` 各步失败不再静默吞掉：继续尽力清理，最终以 `CodeInternal` 汇总返回。
- 系统参数 `Set` 命中软删行时同时清空 `deleted_at`，避免重载后值丢失。
- 前端补 404 / 错误边界（含 403 与服务器错误文案）、`usePermissions().isLoading`。

### 运维
- `http.Server` 增加 `IdleTimeout` / `WriteTimeout`（`SERVER_IDLE_TIMEOUT` / `SERVER_WRITE_TIMEOUT` / `SERVER_SHUTDOWN_TIMEOUT`）。
- 数据库连接池参数 `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` / `DB_CONN_MAX_IDLE_TIME`。
- 操作日志改为有界队列 + 单 worker 异步落库（满则丢弃并告警），优雅关闭时排空。
- 每请求 `X-Request-Id`（回显并进入 RPC 日志），日志级别 / 格式 `LOG_LEVEL` / `LOG_FORMAT`。
- 可选 CORS：`CORS_ALLOWED_ORIGINS`（前后端分域部署）。
- 二进制新增 `healthcheck` / `version` 子命令；Dockerfile 与 compose 增加 `HEALTHCHECK`；构建 `VERSION` 默认取 `git describe`。
- 新增 GitHub Actions CI（后端 lint + nilaway + race 测试；前端 lint + typecheck + vitest；单二进制构建产物）。
- 前端测试基建（vitest + Testing Library），覆盖 401 单飞刷新与权限判定；Go 侧补齐 `audit` / `param` / `mailer` / `web` / `config` / `clientip` / `plugins` 测试。
- Vite 构建拆分 `charts` / `connect` / `vendor` chunk，`sourcemap: false`；主题初始化脚本外置以满足 CSP。

## [0.4.0] - 2026-08

- 站点设置支持注册开关与新用户默认角色。
- 统一 UI 组件与布局。
- `task new --agent` 可选择性生成 AI CLI 资产。

## [0.3.0]

- 重命名为 ZKit；前端包管理器切换为 bun。
- 菜单拖拽排序；角色权限对话框优化。

## [0.2.0]

- 侧边栏移动端抽屉与折叠状态持久化。
- Taskfile 加载 `.env`，`db:up` 按 `DB_DRIVER` 跳过。

## [0.1.0]

- 初始版本：Go + connectRPC + GORM 后端，React 19 + TanStack 前端，单二进制部署。
