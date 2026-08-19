import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ChevronDownIcon, ChevronRightIcon, RefreshCwIcon } from "lucide-react";
import { Fragment, useMemo, useState } from "react";
import { toast } from "sonner";

import { Can } from "@/components/can";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Api } from "@/gen/zerx/v1/api_pb";
import {
  deleteApi,
  listApis,
  syncApis,
  updateApi,
} from "@/gen/zerx/v1/api-ApiService_connectquery";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/apis")({ component: ApisPage });

function useInvalidate() {
  const qc = useQueryClient();
  return () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listApis, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

const PAGE_SIZE = 20;
const ALL_GROUPS = "__all__";

function ApisPage() {
  const { t } = useI18n();
  const invalidate = useInvalidate();
  const { data, isPending } = useQuery(listApis, {});
  const apis = useMemo(() => data?.apis ?? [], [data]);
  const syncMut = useMutation(syncApis);

  const [keyword, setKeyword] = useState("");
  const [groupFilter, setGroupFilter] = useState(ALL_GROUPS);
  const [page, setPage] = useState(1);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const handleSync = async () => {
    try {
      const res = await syncMut.mutateAsync({});
      toast.success(
        t("apiPage.syncedToast", { added: Number(res.added), removed: Number(res.removed) }),
      );
      await invalidate();
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  // All distinct group names (for the filter dropdown), preserving order.
  const allGroupNames = useMemo(() => {
    const seen: string[] = [];
    for (const api of apis) {
      const g = api.group || "\u2014";
      if (!seen.includes(g)) seen.push(g);
    }
    return seen;
  }, [apis]);

  // Apply keyword + group filters.
  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    return apis.filter((api) => {
      const g = api.group || "\u2014";
      if (groupFilter !== ALL_GROUPS && g !== groupFilter) return false;
      if (!kw) return true;
      return (
        api.procedure.toLowerCase().includes(kw) ||
        api.method.toLowerCase().includes(kw) ||
        api.description.toLowerCase().includes(kw)
      );
    });
  }, [apis, keyword, groupFilter]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount);
  const pageItems = useMemo(
    () => filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE),
    [filtered, safePage],
  );

  // Group the current page's items, preserving order.
  const grouped = useMemo(() => {
    const map = new Map<string, Api[]>();
    for (const api of pageItems) {
      const g = api.group || "\u2014";
      const list = map.get(g);
      if (list) list.push(api);
      else map.set(g, [api]);
    }
    return map;
  }, [pageItems]);
  const groupNames = [...grouped.keys()];

  const resetPage = () => setPage(1);
  const toggleGroup = (name: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  const expandAll = () => setCollapsed(new Set());
  const collapseAll = () => setCollapsed(new Set(groupNames));
  const allCollapsed = groupNames.length > 0 && groupNames.every((n) => collapsed.has(n));

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("apiPage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("apiPage.subtitle")}</p>
        </div>
        <Can code="api:update">
          <Button onClick={() => void handleSync()} disabled={syncMut.isPending}>
            <RefreshCwIcon className="size-4" />
            {t("apiPage.sync")}
          </Button>
        </Can>
      </div>

      <div className="flex shrink-0 flex-wrap items-center gap-3">
        <Input
          className="max-w-xs"
          placeholder={t("apiPage.searchPlaceholder")}
          value={keyword}
          onChange={(e) => {
            setKeyword(e.target.value);
            resetPage();
          }}
        />
        <Select
          value={groupFilter}
          onValueChange={(v) => {
            setGroupFilter(v);
            resetPage();
          }}
        >
          <SelectTrigger className="w-48">
            <SelectValue placeholder={t("apiPage.filterGroup")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_GROUPS}>{t("apiPage.allGroups")}</SelectItem>
            {allGroupNames.map((g) => (
              <SelectItem key={g} value={g}>
                {g}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          onClick={() => (allCollapsed ? expandAll() : collapseAll())}
          disabled={groupNames.length === 0}
        >
          {allCollapsed ? t("apiPage.expandAll") : t("apiPage.collapseAll")}
        </Button>
      </div>

      <Card className="flex min-h-0 flex-1 flex-col gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader><TableRow>
            <TableHead>{t("apiPage.procedure")}</TableHead>
            <TableHead>{t("apiPage.method")}</TableHead>
            <TableHead>{t("apiPage.group")}</TableHead>
            <TableHead>{t("common.description")}</TableHead>
            <TableHead className="text-right">{t("common.actions")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  {t("common.noData")}
                </TableCell>
              </TableRow>
            ) : (
              groupNames.map((groupName) => {
                const isCollapsed = collapsed.has(groupName);
                const rows = grouped.get(groupName) ?? [];
                return (
                  <Fragment key={groupName}>
                    <TableRow
                      className="cursor-pointer bg-muted/50 hover:bg-muted/70"
                      onClick={() => toggleGroup(groupName)}
                    >
                      <TableCell colSpan={5} className="h-9 py-0">
                        <span className="flex items-center gap-2">
                          {isCollapsed ? (
                            <ChevronRightIcon className="size-3.5 text-muted-foreground" />
                          ) : (
                            <ChevronDownIcon className="size-3.5 text-muted-foreground" />
                          )}
                          <span className="text-sm font-medium">{groupName}</span>
                          <span className="text-xs tabular-nums text-muted-foreground">
                            {rows.length}
                          </span>
                        </span>
                      </TableCell>
                    </TableRow>
                    {!isCollapsed &&
                      rows.map((api) => (
                        <TableRow key={String(api.id)}>
                          <TableCell className="font-mono text-xs">
                            <span className="flex items-center gap-2 pl-6">
                              <span aria-hidden className="h-4 w-px shrink-0 bg-border" />
                              {api.procedure}
                            </span>
                          </TableCell>
                          <TableCell className="text-xs">{api.method}</TableCell>
                          <TableCell className="text-xs text-muted-foreground">{api.group || "\u2014"}</TableCell>
                          <TableCell className="max-w-xs truncate text-muted-foreground">
                            {api.description}
                          </TableCell>
                          <TableCell className="text-right">
                            <div className="flex justify-end gap-2">
                              <Can code="api:update">
                                <EditApiDialog api={api} />
                              </Can>
                              <Can code="api:delete">
                                <DeleteApiDialog api={api} />
                              </Can>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                  </Fragment>
                );
              })
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between gap-4 border-t bg-card px-4 py-3"><p className="text-sm text-muted-foreground">{t("users.total", { count: filtered.length })}</p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={safePage <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t("users.previous")}
          </Button>
          <span className="text-sm tabular-nums">
            {t("users.pageOf", { page: safePage, pages: pageCount })}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={safePage >= pageCount}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("users.next")}
          </Button>
        </div></div>
      </Card>
    </div>
  );
}

function EditApiDialog({ api }: { api: Api }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidate();
  const mut = useMutation(updateApi);

  const [description, setDescription] = useState(api.description);
  const [group, setGroup] = useState(api.group);

  const handleOpen = (next: boolean) => {
    if (next) {
      setDescription(api.description);
      setGroup(api.group);
    }
    setOpen(next);
  };

  const submit = async () => {
    try {
      await mut.mutateAsync({ id: api.id, description, group });
      toast.success(t("apiPage.updatedToast"));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">
          {t("common.edit")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("common.edit")}</DialogTitle>
          <DialogDescription className="font-mono text-xs">{api.procedure}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="api-group">{t("apiPage.group")}</Label>
            <Input
              id="api-group"
              value={group}
              onChange={(e) => setGroup(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="api-description">{t("common.description")}</Label>
            <Input
              id="api-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => void submit()} disabled={mut.isPending}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteApiDialog({ api }: { api: Api }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidate();
  const mut = useMutation(deleteApi);

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: api.id });
      toast.success(t("apiPage.deletedToast"));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
        >
          {t("common.delete")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("common.delete")}</DialogTitle>
          <DialogDescription className="font-mono text-xs">{api.procedure}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={mut.isPending}
            onClick={() => void handleDelete()}
          >
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
