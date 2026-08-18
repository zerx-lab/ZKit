<div align="center">

# ZKit

**生产可部署、AI 友好的全栈后台管理脚手架**

Go · connectRPC · GORM ·  React 19 · TanStack · Tailwind v4

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=white)](web/package.json)
[![connectRPC](https://img.shields.io/badge/connectRPC-v1-FF5C00)](https://connectrpc.com)
[![Image](https://img.shields.io/badge/Docker_image-~44MB-2496ED?logo=docker&logoColor=white)](Dockerfile)

</div>

---

一套 **单二进制即可上线** 的后台管理底座:前端产物经 `go:embed` 内嵌进后端,`CGO_ENABLED=0` 全静态编译,产出约 44 MB 的 distroless 镜像。proto 是唯一契约来源,前后端类型与校验全部由它生成。

## ✨ 特性

- **声明式契约** — proto 单一来源,生成 Go / TypeScript 类型、connectRPC client 与 protovalidate 校验。
- **类型安全全链路** — 后端 GORM 泛型 API + CLI 代码生成;前端 connect-query + Zod 4。
- **开箱认证授权** — JWT(会话级 jti)+ 最小 RBAC;授权由 Casbin 拦截器统一裁决,handler 零侵入。
- **无默认账号** — 首位注册用户自动成为管理员,安全合规。
- **完善后台 UI** — 侧边栏布局、暗/亮主题、中英(zh/en)i18n、表格 / 表单 / 仪表盘。
- **多数据源** — PostgreSQL / MySQL / SQLite(纯 Go 驱动,无 CGO)。
- **质量门禁** — nilaway 空指针分析、govet nilness、golangci-lint、严格 TypeScript。
- **一键化** — Taskfile 收敛全部开发 / 构建 / 部署命令。

## 🧱 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go 1.26、connectRPC(h2c)、GORM v1.31 + GORM CLI、protovalidate、Casbin |
| 前端 | React 19、Vite、TanStack Router / Query / Table / Form、Radix(shadcn/ui)、Tailwind v4、Zod 4 |
| 数据 | PostgreSQL / MySQL / SQLite |
| 打包 | `go:embed` 内嵌 SPA + distroless 静态二进制 |

## 🚀 快速开始

> 前置:Go 1.26+、[Bun](https://bun.sh)、[Task](https://taskfile.dev)、buf、Docker。

```bash
# 首次设置:装工具/依赖 → 生成代码 → 创建 .env → 启动 dev PostgreSQL
task sync

# 启动开发环境(两个终端各跑一个,Ctrl+C 停止;无热重载,改代码后重跑)
task dev:backend   # 后端(含 db:up + go run)
task dev:web       # 前端 Vite(:5173 代理到 :8080)
```

打开浏览器访问 `http://localhost:5173`,在 `/register` 注册首个用户(即管理员)。

## 🧬 基于本模板创建新项目

用内置 CLI `zkit` 从本模板克隆并改名,一键生成独立可运行的新项目(改 Go module 路径、二进制名、Docker 镜像/卷名、前端包名、品牌显示名、默认库名、localStorage key 前缀)。

**两种用法:**

```bash
# 1) 仓库内开发:在本仓库根目录跑(task 自动传 --from 仓库根)
task new -- github.com/acme/foo ../foo --agent omp [--brand Foo] [--db foo]

# 2) 全局安装(无需先 clone):
go install github.com/zerx-lab/zkit/cmd/zkit@latest
zkit new github.com/acme/foo ../foo --agent omp [--brand Foo] [--db foo]
```

> 提示:`zkit help` 查看命令列表、`zkit --version` 查看版本、`zkit help new` 查看 `new` 用法。

- 位置参数:新 module 路径(必填)、目标目录(可选,缺省 `./<module 末段>`)。
- `--brand` 缺省=module 末段;`--db` 缺省=经 sanitize 的 module 末段;`--from <dir>` 显式指定模板根(跳过缓存)。
- `--agent <name>` 选择新项目保留的 AI CLI 资产:`omp` / `pi` / `claude` / `opencode` / `codex`;`none` 全部移除;`all` 全部保留。缺省为 `all`,兼容既有脚本与生成结果。具体 agent 会保留公共 `AGENTS.md` 与 `.agents/skills`;pi / Claude / OpenCode 还会保留其配置依赖的 `tools/arch-guard`。从已裁剪的派生项目继续创建时,若所选 agent 资产已不存在,CLI 会在写入目标目录前明确报错。
- **模板来源(三态)**:`--from` 指定 → 用该目录;`go run`(仓库内 `(devel)` 态)→ 用当前目录;`go install` 的二进制 → 按其版本把模板克隆/checkout 到 `~/.ZKit/<version>` 再用。**CLI 版本与模板版本同 tag 锁定**(`@v1.2.0` 装的二进制生成时模板也 checkout `v1.2.0`,tag 不可变,几乎免联网);发版前的伪版本则跟远程默认分支最新提交。缓存视为只读镜像,更新时 `reset --hard` 丢弃任何手改;离线无法检查更新时回退缓存并提示。需要本机有 `git`。
- **proto 包名 `zerx.v1` 保留不改**(内部 RPC 命名空间,终端用户不可见;改它需重跑 codegen)。
- 生成码(`*.pb.go` / `*_pb.ts`)逐字节拷贝,其内嵌 descriptor 残留的旧元数据在运行期无害,任意一次 `task gen` 即自愈。

新项目**仅需 `go build` 即可编译**(创建时无需 buf/protoc 工具链)。首跑:

```bash
cd ../foo
cp .env.example .env      # 设置 JWT_SECRET
go build ./...            # 离线编译,无需 codegen
(cd web && bun install && bun run build)
# 完整开发体验(重生成代码、起 dev DB):task sync && task dev
```

推荐显式传 `--agent <name>` 只带目标 CLI 所需资产;需要无 AI 配置的纯项目时传 `--agent none`。

## 📦 构建与部署

```bash
task build        # 本机单二进制(内嵌 SPA)→ bin/zkit
task build:dist   # 静态 linux/amd64 二进制 → bin/zkit-linux-amd64
task docker:build # 构建 distroless 镜像
task docker:up    # 起整套 compose(app + postgres)
```

生产运行需设置 `JWT_SECRET`(缺失则启动失败)。

## 🛠 常用命令

| 命令 | 作用 |
|---|---|
| `task new` | 基于本模板创建新项目(改 module / 品牌 / 库名) |
| `task gen` | 生成全部代码(proto Go/TS + GORM 查询) |
| `task lint` | 后端 golangci-lint + nilaway;前端 ESLint + tsc |
| `task test` | 运行后端测试 |
| `task deps:update` | 升级全部依赖(自动快照,可 `deps:rollback` 回退) |

提交前请执行 `task lint && task test`。

## 🗺 架构

```
浏览器 SPA ──/api/...──► connectRPC handler
                          │
              拦截器链:日志 → 认证 → 校验 → recover
                          │
                       service ──► GORM ──► 数据库
```

生产为单二进制同源部署;开发用 Vite 代理(`:5173` → `:8080`)。

## 📁 目录概览

```
proto/         唯一契约来源(zerx/v1/*.proto)
gen/           生成的 Go 代码(提交入库)
cmd/server/    入口:config → db → migrate → seed → serve
internal/      config / database / model / auth / casbin / service / server ...
web/           React 前端;src/gen/ 为生成的 TS 代码
```

生成代码全部提交入库,Docker / CI / 日常构建**不重跑 codegen**。

## 🤝 二次开发

仓库内置面向 AI 与人类的开发指南,详见 [`AGENTS.md`](AGENTS.md) 及 `.agents/skills/`(后端、前端、授权、安全、模块脚手架等专题 skill)。
