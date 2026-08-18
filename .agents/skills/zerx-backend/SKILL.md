---
name: zerx-backend
description: "ZKit 后端开发规约(Go + connectRPC + GORM)。当新增/修改 RPC、实现 service handler、写数据访问、用 GORM 泛型 API、自定义 querier SQL、跑 codegen 时使用。Keywords: connectRPC, handler, service, GORM, gorm.G, 泛型, Count, FirstOrCreate, nilaway, getter, querier, 自定义 SQL, deleted_at, 软删, convert, proto, task gen, buf, 代码生成, JS 保留字, 后端, backend, Go"
---
# ZKit 后端开发规约

> Claude 已熟悉 Go / GORM / connectRPC;以下是 ZKit 特有规则。

## 新增一个 RPC(流程)
1. 编辑 `proto/zerx/v1/*.proto`,加 message / rpc。**方法名避开 JS 保留字**。校验约束写在字段上:`string email = 1 [(buf.validate.field).string.email = true];`(亦有 `min_len`、`pattern:"^[a-z][a-z0-9_]*$"`)。
2. `task gen` → 产出 Go handler 接口 + TS 类型 + connect-query hook。
3. 在 `internal/service/` 实现 handler,**照抄 `user_service.go` 范式但不写 `RequireRole`**(授权交 Casbin,见 `skill://zerx-authz`)。
4. 注册:`internal/server/server.go` 用 `reg(zerxv1connect.NewXxxServiceHandler(service.NewXxx(...), opts))`。免认证 procedure 进 `public`,已登录即放行进 `selfServe`。注意 `assertServicesRegistered` 会校验所有 `zerx.v1` service 都已挂载(漏挂会启动失败)。
5. 前端用 connect-query hook(见 `skill://zerx-frontend`)。

## service 范式(以 `internal/service/user_service.go` 为模板转写)
- 结构:`type XxxService struct { db *gorm.DB }` + `var _ zerxv1connect.XxxServiceHandler = (*XxxService)(nil)` + `NewXxxService(db)`。
- 签名:`func (s *XxxService) Method(ctx, req *connect.Request[zerxv1.XReq]) (*connect.Response[zerxv1.XResp], error)`。
- 返回:`connect.NewResponse(&zerxv1.XResp{...})`;转换用 `convert.go` 的 `toProto<Struct>(model.X) *zerxv1.X`(命名 `toPro+原型 struct`,列表 `toProto<Struct>s`)。
- **含媒体 URL 的转换多传 `*media.Media`**:`toProtoFile(f, m)`(`Url=m.ResolveFile(f.Key,f.Visibility)`)、`toProtoUser(u, roles, totp, m)`(`Avatar=m.ResolveAvatar(u.Avatar)`)。service 结构体持 `media *media.Media` 字段,经 `NewXxxService(..., m)` 注入(见 `server.go` 的 `mediaResolver`);blob 鉴权/签名 URL 详见 `skill://zerx-security`。
- 错误映射:`gorm.ErrRecordNotFound → CodeNotFound`;业务冲突 → `CodeAlreadyExists`;内部 → `CodeInternal`;无权限由拦截器返 `CodePermissionDenied`(handler 不主动返)。

## GORM 坑表
| 错误写法 | 正解 |
|---|---|
| `gorm.G[T](db).Count(ctx)` | `gorm.G[T](db).Count(ctx, "id")`(必须传列名) |
| `if err == gorm.ErrRecordNotFound` | `errors.Is(err, gorm.ErrRecordNotFound)` |
| 泛型 `First` 取 `.Error` 字段 | 泛型 `First` 返回 `(T, error)`,**无 `.Error`**;已无 `FirstOrCreate`/`Save` |
| `Updates(struct{Status:false})`(跳零值,写不了 false) | `db.Model(&T{}).Where(...).Updates(map[string]any{"status": false})` |
| 为基础查询跑 codegen | 默认查询用 `gorm.G[T](db).Where(...).First/Find/Create(ctx)`,免 codegen |
| 直接取 `req.Msg.Email`(nilaway 标记) | 用 getter `req.Msg.GetEmail()`(空安全) |

- 基础范式:`gorm.G[model.X](db).Where("col = ?", v).First(ctx)` / `.Order(...).Limit(...).Offset(...).Find(ctx)` / `.Create(ctx, &x)`。
- 自定义查询走 `query.Query[model.X](db).Method(ctx, args)`(见下)。

## nilaway
- `task lint` 跑 nilaway,已 `-exclude-pkgs` 排除 `gen`、`internal/query`。
- 读 proto 字段一律用 getter,确保空安全可被识别。

## 自定义 querier(`internal/model/querier.go`)
- 在 `Query[T any]` 接口的方法上写 SQL 注释,`task gen`(或 `task gen:db`)生成进 `internal/query`。
- 规则:绑定参数 `@name`(非 DB 特定字符串函数,保持跨 sqlite/postgres/mysql 可移植);字符串单引号;**raw SQL 不走软删,需显式 `AND deleted_at IS NULL`**。
- 例:`// SELECT * FROM @@table WHERE name LIKE @keyword AND deleted_at IS NULL` → `SearchByName(keyword string) ([]T, error)`,调用 `query.Query[model.User](db).SearchByName(ctx, "%"+kw+"%")`。

## 模型字段块(`internal/model/*.go`)
`ID uint64 [gorm:"primaryKey"]` … `CreatedAt/UpdatedAt time.Time` … `DeletedAt gorm.DeletedAt [gorm:"index"]`。迁移走 **gormigrate**(`internal/database/migrate.go`,记录表 `migrations`):新模型加进 `0001_baseline` 的 `tx.AutoMigrate(...)` 快照列表(增量:既存库自动补缺表/列)。`casbin_rule` 不在此处(gorm-adapter 在 `server.New` 内自动迁移)。需数据回填/删列时另加 `000N_*` 迁移并用 `tx.Migrator().HasColumn(...)` 守卫。

## codegen 版本同步
- `buf.gen.yaml` 内 Go 插件的 `@version`(`protoc-gen-go`、`protoc-gen-connect-go`)需与 `go.mod` 对应库版本**手动一致**;升级库后同步改字符串再 `task gen`。
- TS 走 `buf.gen.web.yaml` + `--include-imports`(为生成 `buf/validate/validate_pb.ts`);Go 生成**不带** `--include-imports`(protovalidate 来自 Go module,避免重复 WKT)。
- `connectrpc.com/validate` 为 unstable,升级前先读其 CHANGELOG。

## 源码锚点
`internal/service/user_service.go`(`ListUsers`/`CreateUser`)、`internal/service/convert.go`、`internal/model/{user.go,querier.go}`、`internal/database/migrate.go`、`proto/zerx/v1/user.proto`、`internal/server/server.go`。
