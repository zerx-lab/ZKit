---
name: zerx-frontend
description: "ZKit 前端开发规约(React 19 + TanStack + connect-query + Zod4 + Tailwind v4)。当新增/修改路由页、CRUD 列表/表单、调用 connect-query hooks、做缓存失效、i18n、主题时使用。Keywords: React, TanStack Router, react-query, useQuery, useMutation, connect-query, createConnectQueryKey, invalidateQueries, react-form, Zod, Tailwind v4, bigint, uint64, table v8, i18n, 主题, theme, 语义 token, Can, permissions, gcTime, placeholderData, 前端, frontend"
---
# ZKit 前端开发规约

> Claude 已熟悉 React 19 / TanStack / Zod;以下是 ZKit 特有规则。最全范式页:`web/src/routes/_authed/users.tsx`(次选 `params.tsx`)。

## 数据访问与失效
- hook 导入:`import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query"`。
- method 导入路径形状:`@/gen/zerx/v1/<entity>-<Service>_connectquery`,如 `@/gen/zerx/v1/user-UserService_connectquery`(导出 `listUsers/createUser/updateUser/deleteUser`)。注意 entity 前缀按文件名,如 `site-SiteSettingsService_connectquery`。
- 查询:`useQuery(listUsers, { page: { page, pageSize }, keyword })`;无入参 `useQuery(listRoles)`。
- 变更:`const m = useMutation(createUser); await m.mutateAsync(value)`;loading 用 `m.isPending`。
- 失效(**确切对象参数**):
  ```ts
  queryClient.invalidateQueries({
    queryKey: createConnectQueryKey({ schema: listUsers, cardinality: "finite" }),
  });
  ```

## 坑表
| 错误写法 | 正解 |
|---|---|
| `cacheTime` / `isLoading` | react-query v5:`gcTime` / `isPending` / `placeholderData` |
| `tailwindcss-animate` | Tailwind v4 动画用 `tw-animate-css` |
| 改 `tailwind.config.js` / `postcss.config.js` | 二者**不存在**;主题变量在 `web/src/styles.css` |
| `z.string().email()` | Zod4 顶层格式函数 `z.email()` |
| 期望 `onSubmit` 拿到 transform 后值 | Standard Schema **不 transform**,拿到的是输入值;object 级 `.refine/.check` 仅基础字段通过后运行 |
| 拦截器数组首项先执行 | connect `Interceptor` 数组**末尾先执行**(洋葱) |
| `id` 当 number | `uint64`→`bigint`;显示 `String(id)`,列/key 用 `String(info.getValue())` |
| 用 shadcn `form` / react-hook-form | 表单用 `@tanstack/react-form` |
| 依赖 `exactOptionalPropertyTypes` | 已**关闭**(与 shadcn/Radix 不兼容);其余严格 flag(`strict`/`noUncheckedIndexedAccess`/`noUnused*`)保留 |

## 表格(react-table v8)
`const columnHelper = createColumnHelper<User>()` → `columnHelper.accessor("col", { header, cell })` / `columnHelper.display({ id, header, cell })` → `useReactTable({ data, columns, getCoreRowModel: getCoreRowModel() })` → `flexRender`。

## 表单(react-form)
`useForm({ defaultValues, validators: { onChange: zodSchema }, onSubmit: async ({ value }) => … })`;`<form.Field name="...">`;错误用 `firstErrorMessage(field.state.meta.errors)`(`web/src/lib/form.ts`);可复用字段组件用 `AnyFieldApi` 类型。

## i18n(`web/src/lib/i18n.tsx`)
- 结构锁:`const en = {...}; const zh: typeof en = {...}`。新增文案 **en/zh 同步加同名 key**(缺 key 编译失败)。
- 用法:`const { t, locale, setLocale, toggleLocale } = useI18n(); t(key, params?)`;缺失回退 en 再回退 key。

## 主题(`web/src/lib/theme.tsx`)
- `const { theme, setTheme, toggleTheme } = useTheme()`。
- 样式一律用**语义 token**(`bg-background` / `text-foreground` 等,定义于 `web/src/styles.css` 的 `:root`/`.dark`,`@theme inline` 映射),**勿写死颜色**。
- Toaster 主题由 `__root.tsx` 透传 `useTheme()`。

## 权限显隐(纯 UX,非安全边界)
- `<Can code="user:create">…</Can>`(组件 `web/src/components/can.tsx`,props `{ code, children }`)。
- 判定源 `web/src/lib/permissions.tsx`:`usePermissions().can(code)` = `roles.includes("admin") || codes.has(code)`(`roles: string[]` 多角色;接口 `PermissionContextValue{ roles, can }`;`me` 返 `user.roles`);数据来自 `me` + `getUserButtons` query。
- 真正鉴权在同名 procedure 的 Casbin 策略上,见 `skill://zerx-authz`。

## lib 速查
- `transport.ts`:`transport`、`authedFetch(input, init?)`(401 single-flight 刷新→重试一次→失败清 token 跳登录)。
- `query-client.ts`:`queryClient`(staleTime 30s, gcTime 5min, retry 1)。
- `auth.ts`:`getAccessToken/setTokens/clearTokens/isAuthenticated/auth`。
- `menu-icons.ts`:`iconByName: Record<string, LucideIcon>`、`menuIcon(name)`(fallback `CircleIcon`)。
- 生成物命名:`web/src/gen/zerx/v1/<entity>_pb.ts`、`<entity>-<Service>_connectquery.ts`、`web/src/gen/buf/validate/validate_pb.ts`。
- 路由:`web/vite.config.ts` 用 `tanstackRouter({ target:"react", autoCodeSplitting:true })` 生成并提交 `src/routeTree.gen.ts`。

## 源码锚点
`web/src/routes/_authed/users.tsx`、`web/src/components/can.tsx`、`web/src/lib/{permissions.tsx,i18n.tsx,transport.ts,theme.tsx,query-client.ts,form.ts,auth.ts,menu-icons.ts}`。
