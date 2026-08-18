---
description: "按钮默认不透明白底:禁止在 Button 上覆写 bg-background/bg-transparent;页级操作用 outline,ghost 仅明确要透明时用"
scope: "tool:edit, tool:write"
globs:
  - "web/src/routes/**/*.tsx"
  - "web/src/components/**/*.tsx"
  - "web/src/plugin-components/**/*.tsx"
interruptMode: never
condition: "<Button[^>]*(?:bg-transparent|bg-background)"
---

检测到给 `Button` 覆写了透明/页面底色(`bg-transparent` / `bg-background`)。页级功能按钮必须不透明白底,与灰底页面区分。

## 必须这样做
- 次操作(导出/清理/搜索/分页/取消):`variant="outline"`(`button.tsx` 已是 `bg-card`)。
- 主操作:`variant="default"`;危险确认:`variant="destructive"`。
- **只有**明确要透明时才用 `variant="ghost"`(顶栏 icon、表格行内编辑删除);不要再叠 `bg-transparent`/`bg-background`。

完整规约见 `skill://zerx-frontend` 按钮节。
