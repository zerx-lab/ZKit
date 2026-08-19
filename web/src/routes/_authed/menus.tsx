import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  ChevronDownIcon,
  ChevronRightIcon,
  GripVerticalIcon,
  InfoIcon,
  PlusIcon,
} from "lucide-react";
import { Fragment, useMemo, useRef, useState, type DragEvent } from "react";
import { toast } from "sonner";

import { Can } from "@/components/can";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Menu, MenuButton } from "@/gen/zerx/v1/menu_pb";
import type { PluginInfo } from "@/gen/zerx/v1/plugin_pb";
import { listPlugins } from "@/gen/zerx/v1/plugin-PluginService_connectquery";
import {
  createMenu,
  createMenuButton,
  deleteMenu,
  deleteMenuButton,
  getUserMenus,
  listMenus,
  reorderMenus,
  updateMenu,
} from "@/gen/zerx/v1/menu-MenuService_connectquery";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/_authed/menus")({ component: MenusPage });

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

const PAGE_SIZE = 8;

interface FlatMenu {
  menu: Menu;
  depth: number;
}

function menuPlugin(menu: Menu, plugins: PluginInfo[]): PluginInfo | undefined {
  for (const plugin of plugins) {
    const prefix = `plg_${plugin.name}`;
    if (menu.name === prefix || menu.name.startsWith(`${prefix}_`)) {
      return plugin;
    }
  }
  return undefined;
}

function pluginMenuLocked(plugin: PluginInfo | undefined): boolean {
  return plugin !== undefined && (!plugin.enabled || plugin.pendingRemoval);
}

// Flatten a single top-level menu subtree (root included) into depth-tagged rows.
function flattenTree(menu: Menu, depth = 0, out: FlatMenu[] = []): FlatMenu[] {
  out.push({ menu, depth });
  for (const child of menu.children) flattenTree(child, depth + 1, out);
  return out;
}

function findSiblings(roots: Menu[], parentId: bigint): Menu[] {
  if (parentId === 0n) return roots;
  const walk = (nodes: Menu[]): Menu[] | null => {
    for (const n of nodes) {
      if (n.id === parentId) return n.children;
      const hit = walk(n.children);
      if (hit) return hit;
    }
    return null;
  };
  return walk(roots) ?? [];
}

function arrayMove<T>(arr: readonly T[], from: number, to: number): T[] {
  const next = arr.slice();
  const [item] = next.splice(from, 1);
  if (item === undefined) return next;
  next.splice(to, 0, item);
  return next;
}

// True if the menu subtree contains the keyword in any title or path.
function matchesKeyword(menu: Menu, kw: string): boolean {
  if (
    menu.title.toLowerCase().includes(kw) ||
    menu.path.toLowerCase().includes(kw)
  ) {
    return true;
  }
  return menu.children.some((c) => matchesKeyword(c, kw));
}

function DragHandle({ enabled, label }: { enabled: boolean; label: string }) {
  if (!enabled) return null;
  return (
    <span
      className="inline-flex cursor-grab text-muted-foreground active:cursor-grabbing"
      aria-label={label}
      onClick={(e) => e.stopPropagation()}
    >
      <GripVerticalIcon className="size-4" />
    </span>
  );
}

function MenuActionCells({
  menu,
  invalidate,
  locked,
}: {
  menu: Menu;
  invalidate: () => void;
  locked: boolean;
}) {
  const { t } = useI18n();

  return (
    <div className="flex justify-end gap-2" onClick={(e) => e.stopPropagation()}>
      {locked ? (
        <span className="text-xs text-muted-foreground">{t("menuPage.enablePluginToManage")}</span>
      ) : (
        <>
          <Can code="menu:update">
            <MenuButtonsDialog menu={menu} onDone={invalidate} />
            <MenuDialog mode="edit" menu={menu} onDone={invalidate} />
          </Can>
          <Can code="menu:delete">
            <DeleteMenuDialog menu={menu} onDone={invalidate} />
          </Can>
        </>
      )}
    </div>
  );
}

function MenusPage() {
  const { t } = useI18n();
  const { can } = usePermissions();
  const qc = useQueryClient();
  const { data, isPending } = useQuery(listMenus);
  const { data: pluginData } = useQuery(listPlugins, {});
  const topMenus = useMemo(() => data?.menus ?? [], [data]);
  const plugins = useMemo(() => pluginData?.plugins ?? [], [pluginData]);
  const reorderMut = useMutation(reorderMenus);

  const [keyword, setKeyword] = useState("");
  const [page, setPage] = useState(1);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [dragging, setDragging] = useState<{ id: bigint; parentId: bigint } | null>(null);
  const [overId, setOverId] = useState<bigint | null>(null);
  const draggingRef = useRef<{ id: bigint; parentId: bigint } | null>(null);
  const didDrag = useRef(false);

  const canDrag = can("menu:update") && keyword.trim() === "";

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: createConnectQueryKey({ schema: listMenus, cardinality: "finite" }) });
    void qc.invalidateQueries({ queryKey: createConnectQueryKey({ schema: getUserMenus, cardinality: "finite" }) });
  };

  const applyReorder = async (parentId: bigint, fromId: bigint, toId: bigint) => {
    if (fromId === toId) return;
    const siblings = findSiblings(topMenus, parentId);
    const from = siblings.findIndex((s) => s.id === fromId);
    const to = siblings.findIndex((s) => s.id === toId);
    if (from < 0 || to < 0) return;
    const next = arrayMove(siblings, from, to);
    try {
      await reorderMut.mutateAsync({ parentId, ids: next.map((s) => s.id) });
      toast.success(t("menuPage.reorderedToast"));
      invalidate();
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  const rowDrag = (menu: Menu, enabled = true) => {
    if (!canDrag || !enabled) return {};
    return {
      draggable: true,
      onDragStart: (e: DragEvent) => {
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", String(menu.id));
        const payload = { id: menu.id, parentId: menu.parentId };
        draggingRef.current = payload;
        setDragging(payload);
        didDrag.current = false;
      },
      onDragOver: (e: DragEvent) => {
        const d = draggingRef.current;
        if (!d || d.parentId !== menu.parentId || d.id === menu.id) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        didDrag.current = true;
        setOverId(menu.id);
      },
      onDrop: (e: DragEvent) => {
        e.preventDefault();
        const d = draggingRef.current;
        if (!d || d.parentId !== menu.parentId) return;
        void applyReorder(menu.parentId, d.id, menu.id);
        draggingRef.current = null;
        setDragging(null);
        setOverId(null);
      },
      onDragEnd: () => {
        draggingRef.current = null;
        setDragging(null);
        setOverId(null);
      },
    };
  };

  const dropClass = (id: bigint) =>
    overId === id && dragging && dragging.id !== id ? "bg-primary/10" : "";

  // Filter top-level groups whose subtree matches the keyword.
  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    if (!kw) return topMenus;
    return topMenus.filter((m) => matchesKeyword(m, kw));
  }, [topMenus, keyword]);

  const hasInactivePluginMenus = useMemo(
    () =>
      topMenus.some((root) =>
        flattenTree(root).some(({ menu }) => pluginMenuLocked(menuPlugin(menu, plugins))),
      ),
    [topMenus, plugins],
  );

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount);
  const pageGroups = useMemo(
    () => filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE),
    [filtered, safePage],
  );

  const groupKeys = pageGroups.map((m) => String(m.id));
  const resetPage = () => setPage(1);
  const toggleGroup = (key: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  const expandAll = () => setCollapsed(new Set());
  const collapseAll = () => setCollapsed(new Set(groupKeys));
  const allCollapsed = groupKeys.length > 0 && groupKeys.every((k) => collapsed.has(k));

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("menuPage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("menuPage.subtitle")}</p>
        </div>
        <Can code="menu:create">
          <MenuDialog mode="create" onDone={invalidate} />
        </Can>
      </div>

      <div className="flex shrink-0 flex-wrap items-center gap-3">
        <Input
          className="max-w-xs"
          placeholder={t("menuPage.searchPlaceholder")}
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value);
            resetPage();
          }}
        />
        <Button
          variant="outline"
          size="sm"
          onClick={() => (allCollapsed ? expandAll() : collapseAll())}
          disabled={groupKeys.length === 0}
        >
          {allCollapsed ? t("menuPage.expandAll") : t("menuPage.collapseAll")}
        </Button>
        {canDrag ? <p className="text-sm text-muted-foreground">{t("menuPage.dragHint")}</p> : null}
      </div>

      {hasInactivePluginMenus ? (
        <div className="flex items-start gap-2.5 rounded-lg border border-primary/20 bg-primary/5 px-3.5 py-3 text-sm text-muted-foreground">
          <InfoIcon className="mt-0.5 size-4 shrink-0 text-primary" />
          <p>{t("menuPage.disabledPluginHint")}</p>
        </div>
      ) : null}

      <Card className="flex min-h-0 flex-1 flex-col gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8 px-1" />
              <TableHead>{t("menuPage.title_")}</TableHead>
              <TableHead>{t("menuPage.path")}</TableHead>
              <TableHead>{t("menuPage.icon")}</TableHead>
              <TableHead>{t("common.sort")}</TableHead>
              <TableHead>{t("menuPage.hidden")}</TableHead>
              <TableHead className="text-right">{t("common.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                  {t("common.noData")}
                </TableCell>
              </TableRow>
            ) : (
              pageGroups.map((group) => {
                const key = String(group.id);
                const isCollapsed = collapsed.has(key);
                const rows = flattenTree(group);
                const childCount = rows.length - 1;
                const groupPlugin = menuPlugin(group, plugins);
                const groupLocked = pluginMenuLocked(groupPlugin);
                return (
                  <Fragment key={key}>
                    <TableRow
                      className={cn(
                        "cursor-pointer bg-card hover:bg-accent/55",
                        groupLocked && "border-l-2 border-l-muted-foreground/35 bg-muted/65 text-muted-foreground",
                        dropClass(group.id),
                      )}
                      onClick={() => {
                        if (didDrag.current) {
                          didDrag.current = false;
                          return;
                        }
                        toggleGroup(key);
                      }}
                      {...rowDrag(group, !groupLocked)}
                    >
                      <TableCell className="w-8 px-1">
                        <DragHandle enabled={canDrag && !groupLocked} label={t("menuPage.dragHandle")} />
                      </TableCell>
                      <TableCell className="py-2.5 font-semibold">
                        <span className="flex items-center gap-1.5">
                          {isCollapsed ? (
                            <ChevronRightIcon className="size-4 text-muted-foreground" />
                          ) : (
                            <ChevronDownIcon className="size-4 text-muted-foreground" />
                          )}
                          {t(group.title)}
                          {groupPlugin ? (
                            <Badge variant="outline" className="ml-1 rounded font-normal">
                              {t("menuPage.pluginOwned", { name: groupPlugin.name })}
                            </Badge>
                          ) : null}
                          {groupPlugin?.pendingRemoval ? (
                            <Badge variant="outline" className="ml-1 rounded border-destructive/30 bg-destructive/5 font-normal text-destructive">
                              {t("pluginPage.pendingRemoval")}
                            </Badge>
                          ) : groupPlugin && !groupPlugin.enabled ? (
                            <Badge variant="secondary" className="ml-1 rounded font-normal">
                              {t("menuPage.pluginDisabled")}
                            </Badge>
                          ) : null}
                          {childCount > 0 ? (
                            <Badge variant="secondary" className="ml-1 font-normal">
                              {t("menuPage.children", { count: childCount })}
                            </Badge>
                          ) : null}
                        </span>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{group.path}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{group.icon}</TableCell>
                      <TableCell className="text-muted-foreground">{group.sort}</TableCell>
                      <TableCell>{group.hidden ? t("common.yes") : t("common.no")}</TableCell>
                      <TableCell className="text-right">
                        <MenuActionCells menu={group} invalidate={invalidate} locked={groupLocked} />
                      </TableCell>
                    </TableRow>
                    {!isCollapsed &&
                      rows.slice(1).map(({ menu, depth }) => {
                        const plugin = menuPlugin(menu, plugins);
                        const locked = pluginMenuLocked(plugin);
                        return (
                          <TableRow
                            key={String(menu.id)}
                            className={cn(
                              locked && "border-l-2 border-l-muted-foreground/25 bg-muted/20 text-muted-foreground",
                              dropClass(menu.id),
                            )}
                            {...rowDrag(menu, !locked)}
                          >
                            <TableCell className="w-8 px-1">
                              <DragHandle enabled={canDrag && !locked} label={t("menuPage.dragHandle")} />
                            </TableCell>
                            <TableCell>
                              <span className="flex items-center" style={{ paddingLeft: depth * 20 }}>
                                <span
                                  aria-hidden
                                  className="mr-2 inline-block h-4 w-px bg-border"
                                />
                                <span className="font-medium">{t(menu.title)}</span>
                                {menu.path === "" ? (
                                  <Badge variant="secondary" className="ml-2">
                                    {t("nav.management")}
                                  </Badge>
                                ) : null}
                              </span>
                            </TableCell>
                            <TableCell className="font-mono text-xs">{menu.path}</TableCell>
                            <TableCell className="font-mono text-xs">{menu.icon}</TableCell>
                            <TableCell>{menu.sort}</TableCell>
                            <TableCell>{menu.hidden ? t("common.yes") : t("common.no")}</TableCell>
                            <TableCell className="text-right">
                              <MenuActionCells menu={menu} invalidate={invalidate} locked={locked} />
                            </TableCell>
                          </TableRow>
                        );
                      })}
                  </Fragment>
                );
              })
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between gap-4 border-t bg-card px-4 py-3"><p className="text-sm text-muted-foreground">
          {t("common.total", { count: filtered.length })}
        </p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={safePage <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t("common.previous")}
          </Button>
          <span className="text-sm tabular-nums">
            {t("common.pageOf", { page: safePage, pages: pageCount })}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={safePage >= pageCount}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("common.next")}
          </Button>
        </div></div>
      </Card>
    </div>
  );
}

function MenuDialog({ mode, menu, onDone }: { mode: "create" | "edit"; menu?: Menu; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const createMut = useMutation(createMenu);
  const updateMut = useMutation(updateMenu);

  const [parentId, setParentId] = useState(menu ? String(menu.parentId) : "0");
  const [path, setPath] = useState(menu?.path ?? "");
  const [name, setName] = useState(menu?.name ?? "");
  const [title, setTitle] = useState(menu?.title ?? "");
  const [component, setComponent] = useState(menu?.component ?? "");
  const [icon, setIcon] = useState(menu?.icon ?? "");
  const [sort, setSort] = useState(menu?.sort ?? 0);
  const [hidden, setHidden] = useState(menu?.hidden ?? false);

  const submit = async () => {
    try {
      const base = {
        parentId: BigInt(parentId || "0"),
        path,
        name,
        title,
        component,
        icon,
        sort,
        hidden,
      };
      if (mode === "create") {
        await createMut.mutateAsync(base);
        toast.success(t("menuPage.createdToast"));
      } else if (menu) {
        await updateMut.mutateAsync({ id: menu.id, ...base });
        toast.success(t("menuPage.updatedToast"));
      }
      onDone();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {mode === "create" ? (
          <Button>
            <PlusIcon className="size-4" />
            {t("common.add")}
          </Button>
        ) : (
          <Button variant="ghost" size="sm">
            {t("common.edit")}
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{mode === "create" ? t("common.add") : t("common.edit")}</DialogTitle>
          <DialogDescription>{t("menuPage.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-2 gap-4">
          <Field label={t("menuPage.parent")} value={parentId} onChange={setParentId} />
          <Field label={t("common.name")} value={name} onChange={setName} />
          <Field label={t("menuPage.path")} value={path} onChange={setPath} />
          <Field label={t("menuPage.title_")} value={title} onChange={setTitle} />
          <Field label={t("menuPage.component")} value={component} onChange={setComponent} />
          <Field label={t("menuPage.icon")} value={icon} onChange={setIcon} />
          <div className="flex flex-col gap-2">
            <Label htmlFor="sort">{t("common.sort")}</Label>
            <Input id="sort" type="number" value={sort} onChange={(e) => setSort(Number(e.target.value))} />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="hidden">{t("menuPage.hidden")}</Label>
            <Switch id="hidden" checked={hidden} onCheckedChange={setHidden} />
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => void submit()} disabled={createMut.isPending || updateMut.isPending}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      <Input value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

function DeleteMenuDialog({ menu, onDone }: { menu: Menu; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const mut = useMutation(deleteMenu);

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: menu.id });
      toast.success(t("menuPage.deletedToast"));
      onDone();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive">
          {t("common.delete")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("common.delete")}</DialogTitle>
          <DialogDescription>{t(menu.title)}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" disabled={mut.isPending} onClick={() => void handleDelete()}>
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MenuButtonsDialog({ menu, onDone }: { menu: Menu; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const createMut = useMutation(createMenuButton);
  const deleteMut = useMutation(deleteMenuButton);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");

  const add = async () => {
    try {
      await createMut.mutateAsync({ menuId: menu.id, code, name });
      setCode("");
      setName("");
      onDone();
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  const remove = async (b: MenuButton) => {
    try {
      await deleteMut.mutateAsync({ id: b.id });
      onDone();
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">
          {t("menuPage.manageButtons")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("menuPage.manageButtonsTitle", { name: t(menu.title) })}</DialogTitle>
          <DialogDescription>{t("menuPage.buttons")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          {menu.buttons.map((b) => (
            <div key={String(b.id)} className="flex items-center justify-between rounded border px-3 py-2">
              <div className="flex flex-col">
                <span className="font-mono text-xs">{b.code}</span>
                <span className="text-xs text-muted-foreground">{b.name}</span>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => void remove(b)}
              >
                {t("common.delete")}
              </Button>
            </div>
          ))}
          <div className="flex items-end gap-2">
            <div className="flex flex-1 flex-col gap-1">
              <Label>{t("common.code")}</Label>
              <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder="user:create" />
            </div>
            <div className="flex flex-1 flex-col gap-1">
              <Label>{t("common.name")}</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <Button onClick={() => void add()} disabled={createMut.isPending || !code || !name}>
              {t("menuPage.addButton")}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
