---
name: zerx-skill-authoring
description: "为 ZKit 编写/维护/更新 .agents/skills 下 AI skill 的元规约:token 高效、陈述事实而非教学、进步式披露、frontmatter 与描述关键词写法、何时该新增 skill、代码变更后如何定位并同步对应 skill。当用户要新增/修改/审查/更新本仓库的 skill 或 AI 指令文件、或某子系统代码变更需同步 skill 时使用。Keywords: skill, SKILL.md, .agents/skills, 编写规约, 元技能, frontmatter, description, 关键词, 进步式披露, progressive disclosure, token 高效, 上下文, AGENTS.md, prompt, 提示词, 编写技巧, 更新 skill, 维护 skill, 同步, 变更, 调研源码, 代码变更, 漂移, sync, update, maintain"
---
# ZKit Skill 编写规约

> Claude 已熟悉通用写作与 Markdown;以下是本仓库 skill 的特有规则,目标是**降低每轮 token**且**减少行为漂移**。

## Harness 发现机制(omp)
- 路径固定:`.agents/skills/<name>/SKILL.md`,**必须扁平一层**。嵌套 `skills/group/<skill>/SKILL.md` 不被发现。
- 进步式披露:只有 `name`+`description` 进系统提示;正文经 `read skill://<name>` 按需加载;附属资产经 `skill://<name>/<相对路径>` 按需读(禁绝对路径 / `..`)。
- 去重键=skill 名,首个命中胜出;全局 `.omp` skills(优先级 100)会遮蔽同名 → **本项目一律 `zerx-` 前缀**(现有全局 skill 均无此前缀,无冲突)。
- frontmatter 字段:`name`(缺省=目录名)、`description`(必填、决定匹配质量)、可选 `globs` / `hide` / `disableModelInvocation`。

| 错误写法 | 正解 |
|---|---|
| `alwaysApply: true` | 禁用(会始终注入,违背 token 目标);靠 `description` 关键词触发 |
| 嵌套目录分组 | 扁平一层,名前缀区分(`zerx-authz`) |
| 无前缀 skill 名 | 一律 `zerx-` 前缀防遮蔽 |
| 资产用绝对路径 / `..` | `skill://<name>/<相对路径>` |

## 九条编写规约
1. **陈述事实,不教学**:模型已懂通用技术(Go/GORM/React/RBAC 概念)。只写**本项目非显然的决定**——确切路径、符号、签名、字面量、坑。固定范式句:`Claude 已熟悉 X;以下是 ZKit 特有规则。`
2. **指向源码,不整文件复制**:范式写成"以 `path:symbol` 为模板转写",或截 ≤15 行代表片段;不整文件粘贴(随代码腐烂)。
3. **密集格式**:表格 / 清单 / 签名块优先,每行=一个可执行事实;坑用"错误写法/错误码 → 正解"两列表。
4. **description 写法**:一句"当…时使用" + 中英双语关键词清单覆盖触发词。
5. **不与常驻上下文重复**:常驻不变量(架构、授权安全核)留 `AGENTS.md`;过程性 how-to 与详尽坑表入 skill;同一事实不要两处全文。
6. **frontmatter**:`name`(=目录名)、`description`(必填、关键词丰富);不写 `alwaysApply: true`;`globs` 仅可选元数据,不依赖其触发。
7. **命名与布局**:`zerx-` 前缀;`.agents/skills/<name>/SKILL.md` 扁平一层;资产同目录,`skill://<name>/...` 访问。
8. **交叉引用**:skill 间用 `skill://zerx-xxx` 互链(尤其 `zerx-add-module`),不复制彼此正文。
9. **自测**:每个 skill 用 3–5 条代表查询验 description 能否命中;目标是让任务以更少工具调用完成。

## 何时该新增 skill
- 出现**重复的过程性知识**(同一流程被反复解释)→ 压成 skill。
- 某 skill 正文 >~400 行 → 把超大范式拆为 `skill://<name>/<asset>.md` 附属资产按需读,**而非新增顶层 skill**。
- 纯架构 / 安全不变量(写 handler 也不能误违反)→ 留 `AGENTS.md` 常驻,不入 skill。

## 最小 SKILL.md 模板
```
---
name: zerx-<topic>
description: "<当…时使用,一句>。Keywords: <中英关键词,逗号分隔>"
---
# <标题>
> Claude 已熟悉 <通用技术>;以下是 ZKit 特有规则。
## <小节>
| 错误写法 / 错误码 | 正解 |
|---|---|
| … | … |
## 范式
以 `path:symbol` 为模板转写(勿整文件复制)。
```

## 维护已有 skill(代码变更后同步)
当某子系统代码变更、需更新对应 skill 时,按序执行(对应口令如"鉴权改了,更新对应 skill"):
1. **定位**:据变更子系统查下方「变更 → skill 逆向索引」,确定目标 skill(可能多个;主 skill 优先)。
2. **调研**:`read` 目标 skill 末尾「源码锚点」列出的每个文件 / 符号,取实测现状(行号会漂,按符号定位)。
3. **比对**:逐条 diff skill 正文事实 vs 实测;**实测优先**——skill 写错或过期一律以源码为准。
4. **更新**:仅改变化的事实行,保持 house style(表格 / 锚点 / 不整文件复制 / 陈述事实)。新增坑用"错误写法 → 正解"两列表。
5. **防漂移**:若该事实在 `AGENTS.md` 常驻区也有(授权安全核、`JWT_SECRET` 必设、首注册即 admin、`CGO_ENABLED=0`),**同步改 `AGENTS.md`**;详尽清单(如 public/selfServe 全表)只留 skill 一处,`AGENTS.md` 保持抽象 + 指针。
6. **自测**:用 3–5 条代表查询确认 description 仍命中;若新增了触发词义务在 `description` 的 Keywords 补齐。

## 变更 → skill 逆向索引
改了哪个文件 / 子系统,就同步哪些 skill(避免只想到主 skill 而漏掉同样引用它的 skill):

| 改动的文件 / 子系统 | 需同步的 skill |
|---|---|
| `internal/server/server.go`(public/selfServe/拦截器链/handler 注册) | `zerx-authz`(主)、`zerx-backend`、`zerx-security`、`zerx-add-module` |
| `internal/auth/*`(casbin_interceptor / jwt / context / interceptor / policy) | `zerx-authz`、`zerx-security` |
| `internal/casbin/*`(enforcer 封装、SetRolePermissions / SyncApis) | `zerx-authz` |
| `internal/service/*`、`convert.go` | `zerx-backend`、`zerx-add-module` |
| `internal/model/*`、`internal/model/querier.go`、`internal/query/*` | `zerx-backend`、`zerx-add-module` |
| `internal/database/{seed,migrate}.go`、`internal/apispec/apispec.go` | `zerx-add-module`、`zerx-backend` |
| `internal/{ratelimit,captcha,storage,config,mailer,audit}/*`、`internal/server/{audit_interceptor.go,interceptors.go,export.go,import.go,docs.go,httpauth.go}` | `zerx-security` |
| `internal/jobs/*`、`internal/service/job_service.go`、`internal/model/job.go` | `zerx-security`、`zerx-backend` |
| `web/src/routes/_authed/*`、`web/src/lib/*`、`web/src/components/can.tsx` | `zerx-frontend`、`zerx-add-module` |
| `proto/zerx/v1/*`、`buf.gen*.yaml` | `zerx-backend`(+ 对应模块 skill) |
| `.agents/skills/*`(本规约自身) | `zerx-skill-authoring` |

> 索引随 skill 增减维护:**新增/删除 skill 时,务必同步本表与 `AGENTS.md` §9 Skills 索引**(二者是 skill 集合的两份事实,不同步即漂移)。
