# Changelog

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 SemVer。

## [Unreleased]

### Added
- 前端新增通用分页组件 `Pagination`（页码跳转、每页条数选择），统一替换各列表页重复的分页实现（apis / error-logs / files / jobs / login-logs / menus / operation-logs / users）。
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
