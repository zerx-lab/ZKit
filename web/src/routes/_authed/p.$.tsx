import { useQuery } from "@connectrpc/connect-query";
import { createFileRoute } from "@tanstack/react-router";
import { lazy, Suspense, type ComponentType, useMemo } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { getUserMenus } from "@/gen/zerx/v1/menu-MenuService_connectquery";
import type { Menu } from "@/gen/zerx/v1/menu_pb";
import { useI18n } from "@/lib/i18n";

// Single splat route carries every plugin leaf page: /p/<name> for a flat
// plugin and /p/<name>/<sub> for grouped plugins. The component to render is
// resolved from the matched menu node's `component` field (not from the URL),
// so a grouped plugin needs no per-page route file.
export const Route = createFileRoute("/_authed/p/$")({
  component: PluginPage,
});

// modules is the compile-time glob of every plugin page. Keys are root-relative
// absolute paths, e.g. "/src/plugin-components/shop/Products.tsx".
const modules = import.meta.glob("/src/plugin-components/**/*.tsx");

// findByPath walks the menu tree to locate the node whose path matches exactly.
function findByPath(menus: Menu[], path: string): Menu | undefined {
  for (const m of menus) {
    if (m.path === path) {
      return m;
    }
    const child = findByPath(m.children, path);
    if (child) {
      return child;
    }
  }
  return undefined;
}

function PluginPage() {
  const { _splat } = Route.useParams();
  const { t } = useI18n();
  const { data, isPending } = useQuery(getUserMenus);

  // _splat is everything after "/p/" (e.g. "shop" or "shop/products").
  const path = `/p/${_splat ?? ""}`;
  const menu = useMemo(() => findByPath(data?.menus ?? [], path), [data, path]);

  const Lazy = useMemo(() => {
    const component = menu?.component;
    if (!component) {
      return undefined;
    }
    const key = `/src/plugin-components/${component}.tsx`;
    const loader = modules[key];
    if (!loader) {
      return undefined;
    }
    return lazy(loader as () => Promise<{ default: ComponentType }>);
  }, [menu]);

  if (isPending) {
    return <Skeleton className="h-full min-h-64 w-full" />;
  }

  if (!Lazy) {
    return (
      <div className="flex h-64 flex-col items-center justify-center gap-2 text-muted-foreground">
        <p className="text-lg font-medium">{t("common.notFound")}</p>
        <p className="text-sm">{path}</p>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-1 flex-col">
      <Suspense fallback={<Skeleton className="h-full min-h-64 w-full" />}>
        <Lazy />
      </Suspense>
    </div>
  );
}
