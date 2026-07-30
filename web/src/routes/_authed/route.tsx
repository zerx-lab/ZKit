import { useMutation, useQuery } from "@connectrpc/connect-query";
import {
  createFileRoute,
  Link,
  Outlet,
  redirect,
  useLocation,
  useNavigate,
} from "@tanstack/react-router";
import { ChevronDownIcon, LogOutIcon, PanelLeftIcon } from "lucide-react";
import { useEffect, useState } from "react";

import { BrandLogo } from "@/components/brand-logo";
import { LanguageSwitcher } from "@/components/language-switcher";
import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  logout,
  me,
  revokeSession,
} from "@/gen/zerx/v1/auth-AuthService_connectquery";
import type { Menu } from "@/gen/zerx/v1/menu_pb";
import { getUserMenus } from "@/gen/zerx/v1/menu-MenuService_connectquery";
import { auth, getSessionId } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { menuIcon } from "@/lib/menu-icons";
import { useIsMobile } from "@/lib/use-mobile";
import { useSidebarGroups } from "@/lib/use-sidebar-groups";
import { queryClient } from "@/lib/query-client";
import { PermissionProvider } from "@/lib/permissions";
import { SiteProvider, useSite } from "@/lib/site";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/_authed")({
  beforeLoad: ({ context, location }) => {
    if (!context.auth.isAuthenticated()) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: AuthedLayout,
});

function AuthedLayout() {
  return (
    <PermissionProvider>
      <SiteProvider>
        <AuthedShell />
      </SiteProvider>
    </PermissionProvider>
  );
}

const EMPTY_MENUS: Menu[] = [];

const COLLAPSED_KEY = "zerx.sidebar.collapsed";

function getStoredCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

function AuthedShell() {
  const { t } = useI18n();
  const site = useSite();
  const location = useLocation();
  const isMobile = useIsMobile();
  const [collapsed, setCollapsed] = useState(getStoredCollapsed);
  const [mobileOpen, setMobileOpen] = useState(false);
  const { data, isPending } = useQuery(getUserMenus);
  const menus = data?.menus ?? EMPTY_MENUS;
  const { isGroupOpen, toggleGroup, activeGroupId } = useSidebarGroups(menus, location.pathname);

  // The drawer always shows full labels; the icon rail is desktop-only.
  const railCollapsed = !isMobile && collapsed;

  // Persist in an effect so the state updater stays pure (mirrors
  // use-sidebar-groups).
  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSED_KEY, collapsed ? "1" : "0");
    } catch {
      // ignore: persistence is best-effort.
    }
  }, [collapsed]);

  // Close the drawer after navigating so the new page is visible.
  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!mobileOpen) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMobileOpen(false);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [mobileOpen]);

  return (
    <div className="flex h-svh w-full overflow-hidden">
      {mobileOpen && (
        <div
          aria-hidden="true"
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}
      <aside
        {...(isMobile && !mobileOpen && { inert: true })}
        className={cn(
          "flex h-full flex-col border-r border-sidebar-border bg-sidebar duration-200",
          // Mobile: off-canvas drawer; desktop: static rail with width transition.
          "fixed inset-y-0 left-0 z-50 w-60 transition-transform md:static md:z-auto md:translate-x-0 md:transition-[width]",
          mobileOpen ? "translate-x-0" : "-translate-x-full",
          railCollapsed ? "md:w-16" : "md:w-60",
        )}
      >
        <div className="flex h-14 items-center gap-2.5 border-b border-sidebar-border px-4">
          {site.logo ? (
            <img src={site.logo} alt="" className="size-7 shrink-0 rounded object-contain" />
          ) : (
            <BrandLogo className="size-7" />
          )}
          {!railCollapsed && (
            <span className="truncate font-semibold text-sidebar-accent-foreground">
              {site.name || t("app.name")}
            </span>
          )}
        </div>
        <nav className="flex flex-1 flex-col gap-1 overflow-y-auto p-2">
          {isPending ? (
            <div className="flex flex-col gap-2 p-1">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </div>
          ) : (
            menus.map((menu) => (
              <SidebarNode
                key={String(menu.id)}
                menu={menu}
                collapsed={railCollapsed}
                isGroupOpen={isGroupOpen}
                onToggleGroup={toggleGroup}
                activeGroupId={activeGroupId}
              />
            ))
          )}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col bg-background">
        <Header
          collapsed={railCollapsed}
          onToggle={() =>
            isMobile ? setMobileOpen((value) => !value) : setCollapsed((value) => !value)
          }
          menus={menus}
        />
        <main className="flex-1 overflow-auto p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

function SidebarNode({
  menu,
  collapsed,
  isGroupOpen,
  onToggleGroup,
  activeGroupId,
}: {
  menu: Menu;
  collapsed: boolean;
  isGroupOpen: (id: string) => boolean;
  onToggleGroup: (id: string) => void;
  activeGroupId: string | undefined;
}) {
  const { t } = useI18n();
  const Icon = menuIcon(menu.icon);

  // Group heading (no path): a collapsible section.
  if (menu.path === "") {
    const groupId = String(menu.id);
    const children = menu.children.map((child) => (
      <SidebarNode
        key={String(child.id)}
        menu={child}
        collapsed={collapsed}
        isGroupOpen={isGroupOpen}
        onToggleGroup={onToggleGroup}
        activeGroupId={activeGroupId}
      />
    ));

    // Icon-rail (sidebar collapsed): ignore per-group collapse, always show all
    // children flat with a divider; no clickable chevron.
    if (collapsed) {
      return (
        <>
          <div aria-hidden="true" className="my-1 border-t border-sidebar-border" />
          {children}
        </>
      );
    }

    const isOpen = isGroupOpen(groupId);
    const isActiveGroup = activeGroupId === groupId;
    const panelId = `sidebar-group-${groupId}`;

    return (
      <div className="flex flex-col gap-1">
        <button
          type="button"
          onClick={() => onToggleGroup(groupId)}
          aria-expanded={isOpen}
          aria-controls={panelId}
          aria-label={`${t(isOpen ? "common.collapse" : "common.expand")} ${t(menu.title)}`}
          className={cn(
            "flex w-full items-center justify-between px-3 pt-3 pb-1 text-[11px] font-semibold uppercase tracking-wide transition-colors",
            isActiveGroup ? "text-sidebar-foreground" : "text-sidebar-foreground/60",
          )}
        >
          <span className="truncate">{t(menu.title)}</span>
          <ChevronDownIcon
            className={cn(
              "size-3.5 shrink-0 transition-transform duration-200",
              !isOpen && "-rotate-90",
            )}
          />
        </button>
        <div
          id={panelId}
          {...(!isOpen && { inert: true })}
          className={cn(
            "grid transition-[grid-template-rows] duration-200",
            isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
          )}
        >
          <div className="flex min-h-0 flex-col gap-1 overflow-hidden">{children}</div>
        </div>
      </div>
    );
  }

  // Plugin pages share one splat route (/p/$); the typed Link needs the part
  // after "/p/" as the _splat param (supports grouped sub-paths like
  // /p/shop/products). Core menus keep their static path.
  const isPlugin = menu.path.startsWith("/p/");
  const linkProps = isPlugin
    ? ({ to: "/p/$", params: { _splat: menu.path.slice(3) } } as const)
    : ({ to: menu.path } as const);
  const link = (
    <Link
      {...linkProps}
      className={cn(
        "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground transition-colors",
        "hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
        "data-[status=active]:bg-sidebar-accent data-[status=active]:text-sidebar-primary",
        collapsed
          ? "justify-center px-0"
          : "border-l-2 border-transparent data-[status=active]:border-sidebar-primary",
      )}
    >
      <Icon className="size-4 shrink-0" />
      {!collapsed && <span className="truncate">{t(menu.title)}</span>}
    </Link>
  );

  // Collapsed icon-rail: labels are hidden, so surface the title via tooltip
  // (keyboard-focusable, unlike the native title attribute).
  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{link}</TooltipTrigger>
        <TooltipContent side="right" sideOffset={4}>
          {t(menu.title)}
        </TooltipContent>
      </Tooltip>
    );
  }

  return link;
}

function findMenuTitle(menus: Menu[], pathname: string): string | undefined {
  for (const m of menus) {
    if (m.path !== "" && pathname.startsWith(m.path)) {
      return m.title;
    }
    const child = findMenuTitle(m.children, pathname);
    if (child) {
      return child;
    }
  }
  return undefined;
}

function Header({
  collapsed,
  onToggle,
  menus,
}: {
  collapsed: boolean;
  onToggle: () => void;
  menus: Menu[];
}) {
  const { t } = useI18n();
  const location = useLocation();
  const navigate = useNavigate();
  const { data } = useQuery(me);
  const user = data?.user;
  const revokeMutation = useMutation(revokeSession);
  const logoutMutation = useMutation(logout);

  const titleKey = findMenuTitle(menus, location.pathname);
  const title = titleKey ? t(titleKey) : t("app.name");
  const initial = (user?.name || user?.email || "?").charAt(0).toUpperCase();

  const handleLogout = async () => {
    const sid = getSessionId();
    if (sid) {
      try {
        await revokeMutation.mutateAsync({ id: sid });
      } catch {
        // ignore: log out locally regardless.
      }
    }
    auth.clearTokens();
    queryClient.clear();
    void navigate({ to: "/login" });
  };

  const handleLogoutAll = async () => {
    try {
      await logoutMutation.mutateAsync({});
    } catch {
      // ignore
    }
    auth.clearTokens();
    queryClient.clear();
    void navigate({ to: "/login" });
  };

  return (
    <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border bg-card px-4">
      <Button variant="ghost" size="icon" onClick={onToggle} aria-label="Toggle sidebar">
        <PanelLeftIcon className="size-4" />
      </Button>
      <span className="min-w-0 truncate text-sm font-medium">{title}</span>
      {collapsed ? <span className="sr-only">collapsed</span> : null}

      <div className="ml-auto flex items-center gap-1">
        <ThemeToggle />
        <LanguageSwitcher />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="gap-2 px-2">
              {user?.avatar ? (
                <img
                  src={user.avatar}
                  alt={user.name || user.email}
                  className="size-7 rounded-full object-cover"
                />
              ) : (
                <span className="flex size-7 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
                  {initial}
                </span>
              )}
              {user ? (
                <span className="hidden max-w-32 truncate text-sm sm:inline">
                  {user.name || user.email}
                </span>
              ) : null}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>
              <div className="flex flex-col">
                <span className="truncate font-medium">{user?.name}</span>
                <span className="truncate text-xs font-normal text-muted-foreground">
                  {user?.email}
                </span>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => void handleLogout()}>
              <LogOutIcon className="size-4" />
              {t("common.signOut")}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => void handleLogoutAll()}>
              <LogOutIcon className="size-4" />
              {t("common.signOutAll")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
