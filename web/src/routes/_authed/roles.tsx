import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { PlusIcon, ShieldCheckIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { Can } from "@/components/can";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { Api } from "@/gen/zerx/v1/api_pb";
import { listApis } from "@/gen/zerx/v1/api-ApiService_connectquery";
import type { Menu } from "@/gen/zerx/v1/menu_pb";
import { listMenus } from "@/gen/zerx/v1/menu-MenuService_connectquery";
import type { Role } from "@/gen/zerx/v1/role_pb";
import {
  createRole,
  deleteRole,
  getRolePermissions,
  listRoles,
  setRolePermissions,
  updateRole,
} from "@/gen/zerx/v1/role-RoleService_connectquery";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/roles")({ component: RolesPage });

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function RolesPage() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const { data, isPending } = useQuery(listRoles);
  const roles = data?.roles ?? [];

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: createConnectQueryKey({ schema: listRoles, cardinality: "finite" }) });

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("rolePage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("rolePage.subtitle")}</p>
        </div>
        <Can code="role:create">
          <RoleDialog mode="create" onDone={invalidate} />
        </Can>
      </div>

      <Card className="overflow-hidden py-0">
        <Table>
          <TableHeader className="bg-muted">
            <TableRow>
              <TableHead>{t("common.code")}</TableHead>
              <TableHead>{t("common.name")}</TableHead>
              <TableHead>{t("common.description")}</TableHead>
              <TableHead>{t("common.sort")}</TableHead>
              <TableHead className="text-right">{t("common.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : (
              roles.map((r) => (
                <TableRow key={String(r.id)}>
                  <TableCell className="font-mono text-xs">
                    {r.code}
                    {r.builtin ? (
                      <Badge variant="secondary" className="ml-2">
                        {t("common.builtin")}
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell>{r.name}</TableCell>
                  <TableCell className="text-muted-foreground">{r.description}</TableCell>
                  <TableCell>{r.sort}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Can code="role:update">
                        <PermissionsDialog role={r} />
                        <RoleDialog mode="edit" role={r} onDone={invalidate} />
                      </Can>
                      {!r.builtin ? (
                        <Can code="role:delete">
                          <DeleteRoleDialog role={r} onDone={invalidate} />
                        </Can>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}

function RoleDialog({ mode, role, onDone }: { mode: "create" | "edit"; role?: Role; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const createMut = useMutation(createRole);
  const updateMut = useMutation(updateRole);

  const [code, setCode] = useState(role?.code ?? "");
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [sort, setSort] = useState(role?.sort ?? 0);

  const submit = async () => {
    try {
      if (mode === "create") {
        await createMut.mutateAsync({ code, name, description, sort });
        toast.success(t("rolePage.createdToast"));
      } else if (role) {
        await updateMut.mutateAsync({ id: role.id, name, description, sort });
        toast.success(t("rolePage.updatedToast"));
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
          <DialogDescription>{t("rolePage.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="code">{t("common.code")}</Label>
            <Input id="code" value={code} disabled={mode === "edit"} onChange={(e) => setCode(e.target.value)} placeholder="editor" />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="name">{t("common.name")}</Label>
            <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="desc">{t("common.description")}</Label>
            <Input id="desc" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="sort">{t("common.sort")}</Label>
            <Input id="sort" type="number" value={sort} onChange={(e) => setSort(Number(e.target.value))} />
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

function DeleteRoleDialog({ role, onDone }: { role: Role; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const mut = useMutation(deleteRole);

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: role.id });
      toast.success(t("rolePage.deletedToast"));
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
          <DialogDescription>{role.name}</DialogDescription>
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

interface FlatMenu {
  menu: Menu;
  depth: number;
  isGroup: boolean;
}

function flattenMenus(menus: Menu[], depth = 0, out: FlatMenu[] = []): FlatMenu[] {
  for (const m of menus) {
    out.push({ menu: m, depth, isGroup: m.children.length > 0 });
    flattenMenus(m.children, depth + 1, out);
  }
  return out;
}

function PermissionsDialog({ role }: { role: Role }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">
          {t("rolePage.assign")}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] gap-0 overflow-hidden p-0 sm:max-w-[44rem]">
        <DialogHeader className="border-b px-6 py-5">
          <div className="flex items-center gap-2.5">
            <DialogTitle>{t("rolePage.assignTitle")}</DialogTitle>
            <Badge variant="secondary" className="font-normal">
              {role.name}
            </Badge>
          </div>
          <DialogDescription>{t("rolePage.permissionsDescription")}</DialogDescription>
        </DialogHeader>
        {open ? <PermissionsForm role={role} onClose={() => setOpen(false)} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function PermissionsForm({ role, onClose }: { role: Role; onClose: () => void }) {
  const { t } = useI18n();
  const { data: perms } = useQuery(getRolePermissions, { roleCode: role.code });
  const { data: menuData } = useQuery(listMenus);
  const { data: apiData } = useQuery(listApis);
  const setMut = useMutation(setRolePermissions);

  const flatMenus = useMemo(() => flattenMenus(menuData?.menus ?? []), [menuData]);
  const apisByGroup = useMemo(() => {
    const m = new Map<string, Api[]>();
    for (const a of apiData?.apis ?? []) {
      const g = a.group || "default";
      const arr = m.get(g) ?? [];
      arr.push(a);
      m.set(g, arr);
    }
    return m;
  }, [apiData]);

  const [menuIds, setMenuIds] = useState<Set<bigint>>(new Set());
  const [buttonIds, setButtonIds] = useState<Set<bigint>>(new Set());
  const [procedures, setProcedures] = useState<Set<string>>(new Set());
  const isAdmin = role.code === "admin";
  const effectiveProcedureCount = isAdmin ? (apiData?.apis.length ?? 0) : procedures.size;

  useEffect(() => {
    if (perms) {
      setMenuIds(new Set(perms.menuIds));
      setButtonIds(new Set(perms.buttonIds));
      setProcedures(new Set(perms.procedures));
    }
  }, [perms]);

  const toggleMenu = (id: bigint) => {
    setMenuIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const toggleButton = (id: bigint) => {
    setButtonIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };
  const toggleProc = (p: string) => {
    setProcedures((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  };
  const toggleGroup = (group: string, checked: boolean) => {
    setProcedures((prev) => {
      const next = new Set(prev);
      for (const a of apisByGroup.get(group) ?? []) {
        if (checked) next.add(a.procedure);
        else next.delete(a.procedure);
      }
      return next;
    });
  };

  const save = async () => {
    try {
      await setMut.mutateAsync({
        roleCode: role.code,
        menuIds: [...menuIds],
        procedures: [...procedures],
        buttonIds: [...buttonIds],
      });
      toast.success(t("rolePage.savedToast"));
      onClose();
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <div className="flex h-[min(68vh,34rem)] min-h-0 flex-col">
      <Tabs defaultValue="menus" className="min-h-0 flex-1 gap-0">
        <div className="px-6">
          <TabsList className="h-12 w-full justify-start gap-8 rounded-none border-b bg-transparent p-0">
            <TabsTrigger
              value="menus"
              className="group relative h-12 flex-none rounded-none border-0 px-0 text-muted-foreground shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:scale-x-0 after:bg-primary after:transition-transform data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none data-[state=active]:after:scale-x-100"
            >
              <span>{t("rolePage.tabMenus")}</span>
              <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground group-data-[state=active]:bg-primary/10 group-data-[state=active]:text-primary">
                {menuIds.size}
              </span>
            </TabsTrigger>
            <TabsTrigger
              value="apis"
              className="group relative h-12 flex-none rounded-none border-0 px-0 text-muted-foreground shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:scale-x-0 after:bg-primary after:transition-transform data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none data-[state=active]:after:scale-x-100"
            >
              <span>{t("rolePage.tabApis")}</span>
              <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground group-data-[state=active]:bg-primary/10 group-data-[state=active]:text-primary">
                {effectiveProcedureCount}
              </span>
            </TabsTrigger>
            <TabsTrigger
              value="buttons"
              className="group relative h-12 flex-none rounded-none border-0 px-0 text-muted-foreground shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:scale-x-0 after:bg-primary after:transition-transform data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none data-[state=active]:after:scale-x-100"
            >
              <span>{t("rolePage.tabButtons")}</span>
              <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground group-data-[state=active]:bg-primary/10 group-data-[state=active]:text-primary">
                {buttonIds.size}
              </span>
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent
          value="menus"
          className="mx-6 min-h-0 flex-1 overflow-y-scroll py-3 pr-2 [scrollbar-gutter:stable] [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-muted-foreground/40 [&::-webkit-scrollbar-track]:bg-muted/50 [&::-webkit-scrollbar]:w-2.5"
        >
          <div className="flex flex-col gap-0.5 py-1">
            {flatMenus.map(({ menu, depth, isGroup }) => (
              <label
                key={String(menu.id)}
                className={`flex h-9 cursor-pointer items-center gap-2 rounded-md px-2 transition-colors hover:bg-accent ${
                  isGroup ? "mt-2 bg-muted/50 first:mt-0" : ""
                }`}
                style={{ paddingLeft: depth * 22 + 8 }}
              >
                <Checkbox
                  checked={menuIds.has(menu.id)}
                  onCheckedChange={() => toggleMenu(menu.id)}
                />
                <span
                  className={
                    isGroup ? "text-sm font-medium text-foreground" : "text-sm"
                  }
                >
                  {t(menu.title)}
                </span>
              </label>
            ))}
          </div>
        </TabsContent>

        <TabsContent
          value="apis"
          className="mx-6 min-h-0 flex-1 overflow-y-scroll py-3 pr-2 [scrollbar-gutter:stable] [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-muted-foreground/40 [&::-webkit-scrollbar-track]:bg-muted/50 [&::-webkit-scrollbar]:w-2.5"
        >
          <div className="flex flex-col gap-5 py-1">
            {isAdmin ? (
              <div
                role="status"
                className="flex items-start gap-3 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2.5 text-sm"
              >
                <ShieldCheckIcon className="mt-0.5 size-4 shrink-0 text-primary" />
                <div>
                  <p className="font-medium">{t("rolePage.adminApiAccessTitle")}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {t("rolePage.adminApiAccessDescription")}
                  </p>
                </div>
              </div>
            ) : null}
            {[...apisByGroup.entries()].map(([group, groupApis]) => {
              const allChecked = isAdmin || groupApis.every((a) => procedures.has(a.procedure));
              return (
                <div key={group} className="flex flex-col gap-1">
                  <div className="flex h-9 items-center justify-between rounded-md bg-muted/50 px-3">
                    <span className="text-sm font-medium">{group}</span>
                    {isAdmin ? (
                      <span className="flex items-center gap-1.5 text-xs font-medium text-primary">
                        <ShieldCheckIcon className="size-3.5" />
                        {t("rolePage.allApisAccessible")}
                      </span>
                    ) : (
                      <label className="flex items-center gap-2 text-xs">
                        <Checkbox
                          checked={allChecked}
                          onCheckedChange={(v) => toggleGroup(group, v === true)}
                        />
                        {t("rolePage.selectAllGroup")}
                      </label>
                    )}
                  </div>
                  {groupApis.map((a) => (
                    <label
                      key={String(a.id)}
                      className="flex h-9 cursor-pointer items-center gap-2 rounded-md px-3 transition-colors hover:bg-accent"
                    >
                      <Checkbox
                        checked={isAdmin || procedures.has(a.procedure)}
                        disabled={isAdmin}
                        onCheckedChange={() => toggleProc(a.procedure)}
                      />
                      <span className="font-mono text-xs">{a.method}</span>
                      <span className="text-xs text-muted-foreground">{a.description}</span>
                    </label>
                  ))}
                </div>
              );
            })}
          </div>
        </TabsContent>

        <TabsContent
          value="buttons"
          className="mx-6 min-h-0 flex-1 overflow-y-scroll py-3 pr-2 [scrollbar-gutter:stable] [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-muted-foreground/40 [&::-webkit-scrollbar-track]:bg-muted/50 [&::-webkit-scrollbar]:w-2.5"
        >
          <div className="flex flex-col gap-5 py-1">
            {flatMenus
              .filter(({ menu }) => menu.buttons.length > 0)
              .map(({ menu }) => (
                <div key={String(menu.id)} className="flex flex-col gap-1">
                  <span className="flex h-9 items-center rounded-md bg-muted/50 px-3 text-sm font-medium">
                    {t(menu.title)}
                  </span>
                  {menu.buttons.map((b) => (
                    <label key={String(b.id)} className="flex items-center gap-2 rounded px-2 py-1 hover:bg-accent">
                      <Checkbox
                        checked={buttonIds.has(b.id)}
                        onCheckedChange={() => toggleButton(b.id)}
                      />
                      <span className="font-mono text-xs">{b.code}</span>
                      <span className="text-xs text-muted-foreground">{b.name}</span>
                    </label>
                  ))}
                </div>
              ))}
          </div>
        </TabsContent>
      </Tabs>

      <DialogFooter className="border-t px-6 py-4">
        <Button variant="outline" onClick={onClose} disabled={setMut.isPending}>
          {t("common.cancel")}
        </Button>
        <Button onClick={() => void save()} disabled={setMut.isPending}>
          {t("common.save")}
        </Button>
      </DialogFooter>
    </div>
  );
}
