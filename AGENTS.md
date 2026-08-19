# ZKit — AI 开发指南

> 本文件指导在本仓库之上进行的后续开发(人类与 AI 通用)。所有面向用户的叙述请用**简体中文**(与全局规则一致);代码、命令、标识符保持原样。

## 1. 项目概览

生产可部署、AI 友好的全栈后台管理脚手架:

- **后端**:Go + [connectRPC](https://connectrpc.com)(HTTP/1.1 + h2c)、[GORM](https://gorm.io) 泛型 API + GORM CLI 代码生成、多数据源(PostgreSQL / MySQL / SQLite)。
- **前端**:React 19 + Vite + TanStack(Router / Query / Table / Form)+ Radix(shadcn/ui)+ Tailwind v4 + Zod 4;侧边栏管理布局、暗/亮主题、中英(zh/en)i18n。
- **打包**:后端用 `go:embed` 内嵌前端产物;`CGO_ENABLED=0` 全静态二进制 + distroless 镜像(无 glibc,镜像约 44 MB)。
- **契约**:proto 声明式校验(protovalidate)、JWT 认证、最小 RBAC;无默认账号——首次注册的用户自动成为管理员。
- **质量门禁**:Go 空指针静态分析(nilaway)、govet nilness、golangci-lint;前端严格 TypeScript + ESLint。

数据流:浏览器 SPA → `/api/...`(同源)→ connectRPC handler → 拦截器链(日志 → 认证 → 校验 → recover)→ service → GORM → 数据库。生产为单二进制同源部署;开发用 Vite 代理(`:5173` → `:8080`)。

## 2. 目录结构

```
.
├── Taskfile.yml + taskfiles/{backend,frontend,proto,db,deps,docker}.yml  # 一键化
├── buf.yaml / buf.gen.yaml(Go)/ buf.gen.web.yaml(TS)/ buf.lock
├── proto/zerx/v1/{common,auth,user,role,menu,api,dict,param,file,log}.proto  # 唯一契约来源
├── gen/go/zerx/v1/                             # 生成(提交):*.pb.go + zerxv1connect/*.connect.go
├── cmd/server/main.go                          # 入口:config→db→migrate→seed→serve(h2c)
├── internal/
│   ├── config/      # 12-factor 类型化配置(caarlos0/env + godotenv);含 Auth/Storage
│   ├── database/    # Open(多数据源)/ Migrate / Seed(角色/菜单/按钮/API 目录)/ gen.go
│   ├── model/       # GORM 模型 + querier.go(GORM CLI 输入接口)
│   ├── query/       # GORM CLI 生成(提交):Query[T] + 字段助手
│   ├── auth/        # bcrypt / JWT(jti=会话)/ ctx claims / 认证拦截器 / Casbin 接口鉴权拦截器
│   ├── casbin/      # SyncedCachedEnforcer 封装(sub=角色 code, obj=procedure);gorm-adapter
│   ├── captcha/     # base64Captcha 内存验证码(进程内)
│   ├── ratelimit/   # 登录防爆破(内存滑动窗口:验证码阈值/锁定阈值)
│   ├── param/       # 系统参数运行时缓存(进程内)
│   ├── apispec/     # proto 反射枚举 procedure(API 目录种子/同步)
│   ├── storage/     # 对象存储抽象:local(磁盘)/ s3(minio-go)
│   ├── service/     # connectRPC handler 实现 + convert.go
│   ├── server/      # New() 装配:拦截器链 + 操作日志/审计拦截器 + /api 路由 + /api/upload + SPA + /healthz
│   └── web/         # embed.go(go:embed all:dist)+ dist/(前端产物落点)
└── web/
    ├── vite.config.ts / tsconfig*.json / eslint.config.js / components.json
    └── src/
        ├── main.tsx / router.tsx / styles.css / routeTree.gen.ts(生成,提交)
        ├── gen/                # 生成(提交):*_pb.ts + *-<Service>_connectquery.ts + buf/validate/validate_pb.ts
        ├── lib/{transport,query-client,auth,utils,form,theme,i18n}.ts
        ├── components/{ui/,theme-toggle,language-switcher}.tsx  # shadcn + 主题/语言切换
        └── routes/{__root,index,login,register}.tsx + _authed/{route(侧边栏壳),dashboard,users}.tsx
```

**生成代码全部提交入库**(`gen/`、`web/src/gen/`、`internal/query/`、`web/src/routeTree.gen.ts`),因此 Docker / CI / 日常构建**不重跑 codegen**,且 AI 始终能看到完整类型。

## 3. 技术栈与版本

精确版本以 `go.mod` 与 `web/package.json` 为准(由 lockfile 钉死)。关键主版本:

| 领域 | 选型 |
|---|---|
| Go | 1.26;connect v1.19.x、protobuf-go v1.36.x、gorm v1.31.x、gorm/cli v0.2.x、jwt/v5、glebarez/sqlite(纯 Go)、driver/postgres(pgx,纯 Go)、driver/mysql |
| 校验 | protovalidate(connectrpc.com/validate,**unstable,钉精确版本**) |
| 前端 | React 19、TanStack Router/Query v5/Table **v8**/Form v1、protobuf-es v2、connect/connect-web/connect-query v2、Zod **v4**、Tailwind **v4**、Vite v8、TypeScript 6 |
|工具|buf(系统级)、golangci-lint v2 + nilaway(`go.mod` tool 块,`go tool` 调用)、gorm cli(`go install` 到 `./.bin`)|

## 4. 一键命令(Taskfile)

| 命令 | 作用 |
|---|---|
| `task sync` | 首次设置:装工具/依赖 → 生成代码 → `go mod tidy` → 创建 `.env` → 启动 dev PostgreSQL(`docker compose up -d --wait postgres`) |
| `task dev:backend` / `task dev:web` | 各自前台运行(自行在两个终端管理,Ctrl+C 停止),**无热重载/无 TUI**。`dev:backend` 先 `db:up` 再 `go run ./cmd/server`(改代码后重跑即可);`dev:web` 跑 Vite,前端 `:5173` 代理到后端 `:8080`。改 `.proto` 后手动 `task gen` 再重跑后端 |
| `task new` | 基于本模板创建新项目(改 module / 品牌 / 库名);仓库内 `task new -- github.com/acme/foo ../foo [--brand Foo] [--db foo]`;也可 `go install .../cmd/zkit@latest` 全局用。模板来源三态:`--from` 指定 / `go run`(devel)用当前目录 / 安装态按 tag clone 到 `~/.ZKit/<version>`(CLI 与模板同 tag 锁定)。proto 包名 `zerx.v1` 保留;新项目仅需 `go build` |
| `task gen` | 生成全部代码(proto Go/TS + GORM 查询) |
| `task build` | 构建 SPA → 本机单二进制(内嵌 SPA),产物 `bin/zkit[.exe]` |
| `task build:dist` | 构建 SPA → 静态 `linux/amd64` 二进制 `bin/zkit-linux-amd64` |
| `task lint` | 后端 golangci-lint + nilaway;前端 ESLint + `tsc --noEmit` |
| `task test` | `go test ./...`(前端暂无测试) |
| `task run` | 运行已构建二进制(需 `JWT_SECRET`,`.env` 在 dev 即可) |
| `task docker:build` / `docker:up` / `docker:down` / `db:up` / `db:down` | 构建镜像 / 起停整套 compose(app + postgres,数据持久化于命名卷 `zkit_pgdata`)/ 起停本地 dev PostgreSQL。MySQL 在 `docker-compose.yml` 中默认注释,需手动取消注释启用 |
| `task deps:update` / `deps:rollback` | 升级全部依赖(先快照)/ 从快照回退 |

**提交前务必**:`task lint && task test`。

## 5. AI 工作流(如何扩展)

### 授权铁律(必读,常驻安全核)
- **接口鉴权的唯一权威是 Casbin 拦截器**(`internal/auth/casbin_interceptor.go`)。handler **一律不写** `auth.RequireRole(...)`——授权完全由拦截器统一裁决:`sub=角色 code`、`obj=connectRPC procedure`,精确匹配。
- **三层访问控制(顺序固定)**:`public`(免认证)→ `selfServe`(已登录即放行)→ **admin 角色绕过**(始终放行)→ 否则 `enforcer.Enforce(role, procedure)`。两 map 见 `internal/server/server.go`。内置 `admin`/`user` 两角色(code 不可改名)。
- 把任意 procedure(含写操作)授予某角色即**即时生效**;把 RBAC 管理类写操作授予非 admin = 提权,默认仅 admin 拥有(靠绕过)。菜单/按钮(RoleMenu/RoleButton、`<Can code>`)**不走 Casbin**,前者是关联表、后者纯前端 UX 显隐(非安全边界)。
- **完整细节(public/selfServe 清单、拦截器链顺序、决策伪代码、role-as-code 局限)见 `skill://zerx-authz`。**

### 过程性细节按需读 skill(不必常驻)
- 新增 RPC / 新增模型 / GORM 坑 / 自定义 querier / codegen 版本同步 → `skill://zerx-backend`
- 端到端新增管理模块(proto→model→migrate→service→server→页→i18n→seed→routeTree→授权)→ `skill://zerx-add-module`
- 用**插件机制**低侵入扩展模块 / `task new-plugin` 脚手架 / 插件注册校验 / 启用禁用卸载 / 前端动态页 → `skill://zerx-plugin`
- 前端页 / 数据失效 / 表格表单 / i18n / 主题 / 权限显隐 → `skill://zerx-frontend`
- 校验与鉴权 / 三层访问控制全表 → `skill://zerx-authz`
- 认证 / 会话 / 刷新 / 验证码 / 审计 / 上传 / 存储 → `skill://zerx-security`
- 编写/维护本仓库 skill 的规约 → `skill://zerx-skill-authoring`

## 6. 易错点清单

- 后端坑(`Count` 传列名、`gorm.ErrRecordNotFound`、泛型 `First` 无 `.Error`、零值 `Updates` map、proto getter、nilaway)→ `skill://zerx-backend`
- 前端坑(react-query v5 `gcTime`/`isPending`、Tailwind v4、Zod4、洋葱拦截器、`uint64`→`bigint`、react-form、`exactOptionalPropertyTypes` 关闭)→ `skill://zerx-frontend`

## 7. 安全

常驻三条不变量:
- **生产必设 `JWT_SECRET`**:缺失则启动失败(`os.Exit(1)`)。
- **无默认账号**:不 seed 管理员;首次在 `/register` 注册即管理员。公开注册默认开启,可在网站设置关闭;新用户默认角色可配(禁 admin)。
- **纯 Go / CGO-free**:构建 `CGO_ENABLED=0`;新增依赖须纯 Go。

JWT/会话/刷新/验证码/防爆破/审计/上传/存储/进程内状态/IP 来源等机制详见 `skill://zerx-security`;role-as-code 局限见 `skill://zerx-authz`。

## 8. 版本维护

- 升级流程:`task deps:update`(自动快照)→ `task build && task lint && task test` 验证;有问题 `task deps:rollback`。
- codegen 版本同步(`buf.gen.yaml` 的 `@version` 与 `go.mod` 对齐、TS/Go 的 `--include-imports` 差异、`connectrpc.com/validate` unstable)详见 `skill://zerx-backend`。

## 9. 项目 Skills 索引

过程性细节按需 `read skill://<name>`,不必常驻上下文:

| skill | 何时读 |
|---|---|
| `zerx-authz` | 写/改 handler、注册 service、配 public/selfServe、调 Casbin 策略、角色/菜单/按钮权限 |
| `zerx-backend` | 新增/改 RPC、实现 service、GORM 数据访问、自定义 querier、codegen |
| `zerx-frontend` | 新增/改路由页、CRUD 列表/表单、connect-query、缓存失效、i18n、主题 |
| `zerx-security` | 登录注册、token 刷新、会话、限流、审计日志、上传、存储 |
| `zerx-add-module` | 端到端新增一个后台管理模块 / 资源 / CRUD 页 |
| `zerx-plugin` | 编译期插件机制:task new-plugin 脚手架、注册/校验、生命周期、前端动态页 |
| `zerx-skill-authoring` | 新增/修改/审查本仓库的 skill 或 AI 指令文件 |

## 10. 架构合规守卫(arch-guard)

主动拦截层,编辑命中架构违规即把"正确做法"提示词注入模型(硬规则中止重写,软规则随结果折叠提醒)。

### omp 原生 TTSR(核心)
- `.omp/rules/*.md` — 6 条规则(omp Time Traveling Stream Rules,流式写工具参数时实时正则匹配 + 路径门控):
  - `arch-i18n-no-hardcoded-cjk`(软):前端 routes/components 禁硬编码 CJK,走 i18n。
  - `arch-no-requirerole-in-service`(硬中止):service handler 禁 `RequireRole`,授权唯一权威是 Casbin 拦截器。
  - `arch-react-query-v5-naming`(软):`cacheTime` → `gcTime`。
  - `arch-no-hardcoded-tailwind-color`(软):禁写死调色板色值,用语义 token。
  - `arch-gorm-removed-api`(软):禁 `FirstOrCreate`/`.Save(`;`Count` 须传列名。
  - `arch-no-bare-fetch-api`(软):前端 routes/components 禁裸 `fetch("/api/...")`,走 connect-query hook 或 `authedFetch`(含 401 刷新)。
- `.omp/config.yml` — **`edit.mode: replace`**(关键:默认 hashline 的 edit 参数无 path,路径门控对增量编辑失效);`ttsr.repeatMode: after-gap` + `repeatGap: 5`。
- `.omp/WATCHDOG.md` — 可选 advisor(第二模型)语义级 review 清单。

### 跨工具共享(Claude Code / pi / opencode)
- `tools/arch-guard/` — 共享匹配逻辑(`match.mjs` + `patterns.json`)、命令 hook 适配器、opencode 插件、自测。Codex 无法在编辑前拦截文件(其 PreToolUse 只拦 Bash),记为已知缺口。
- **改规则要两处同步**:`.omp/rules/*.md`(富正文)与 `tools/arch-guard/patterns.json`(跨工具正则),正则保持一致。
