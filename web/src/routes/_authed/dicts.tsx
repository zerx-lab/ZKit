import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
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
import type { Dict, DictItem } from "@/gen/zerx/v1/dict_pb";
import {
  createDict,
  createDictItem,
  deleteDict,
  deleteDictItem,
  listDictItems,
  listDicts,
  updateDict,
  updateDictItem,
} from "@/gen/zerx/v1/dict-DictService_connectquery";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/dicts")({ component: DictsPage });

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function DictsPage() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const [selected, setSelected] = useState<Dict | null>(null);

  const { data: dictData, isPending } = useQuery(listDicts, {
    page: { page: 1, pageSize: 100 },
    keyword: "",
  });
  const dicts = dictData?.dicts ?? [];

  const invalidateDicts = () =>
    qc.invalidateQueries({ queryKey: createConnectQueryKey({ schema: listDicts, cardinality: "finite" }) });

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("dictPage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("dictPage.subtitle")}</p>
        </div>
        <Can code="dict:create">
          <DictDialog mode="create" onDone={invalidateDicts} />
        </Can>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-6 lg:grid-cols-2">
        <Card className="flex min-h-0 flex-col overflow-hidden py-0">
          <div className="border-b bg-muted px-4 py-2 text-sm font-medium">{t("dictPage.title")}</div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("common.type")}</TableHead>
                <TableHead>{t("common.name")}</TableHead>
                <TableHead>{t("common.status")}</TableHead>
                <TableHead className="text-right">{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isPending ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                    {t("common.loading")}
                  </TableCell>
                </TableRow>
              ) : dicts.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-24 text-center text-muted-foreground">
                    {t("common.noData")}
                  </TableCell>
                </TableRow>
              ) : (
                dicts.map((d) => (
                  <TableRow
                    key={String(d.id)}
                    onClick={() => setSelected(d)}
                    className={cn("cursor-pointer", selected?.id === d.id && "bg-accent")}
                  >
                    <TableCell className="font-mono text-xs">{d.type}</TableCell>
                    <TableCell>{d.name}</TableCell>
                    <TableCell>
                      <Badge variant={d.status ? "default" : "secondary"}>
                        {d.status ? t("common.enabled") : t("common.disabled")}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right" onClick={(e) => e.stopPropagation()}>
                      <div className="flex justify-end gap-2">
                        <Can code="dict:update">
                          <DictDialog mode="edit" dict={d} onDone={invalidateDicts} />
                        </Can>
                        <Can code="dict:delete">
                          <DeleteDictDialog dict={d} onDone={invalidateDicts} />
                        </Can>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </Card>

        <Card className="flex min-h-0 flex-col overflow-hidden py-0">
          <div className="flex items-center justify-between border-b bg-muted px-4 py-2 text-sm font-medium">
            {t("dictPage.items")}
            {selected ? (
              <Can code="dict:update">
                <DictItemDialog mode="create" dictId={selected.id} />
              </Can>
            ) : null}
          </div>
          {selected ? (
            <DictItemsTable dictId={selected.id} />
          ) : (
            <div className="p-8 text-center text-sm text-muted-foreground">{t("dictPage.selectDict")}</div>
          )}
        </Card>
      </div>
    </div>
  );
}

function DictItemsTable({ dictId }: { dictId: bigint }) {
  const { t } = useI18n();
  const { data, isPending } = useQuery(listDictItems, { dictId });
  const items = data?.items ?? [];

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("common.label")}</TableHead>
          <TableHead>{t("common.value")}</TableHead>
          <TableHead>{t("common.sort")}</TableHead>
          <TableHead>{t("common.status")}</TableHead>
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
        ) : items.length === 0 ? (
          <TableRow>
            <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
              {t("common.noData")}
            </TableCell>
          </TableRow>
        ) : (
          items.map((it) => (
            <TableRow key={String(it.id)}>
              <TableCell>{it.label}</TableCell>
              <TableCell className="font-mono text-xs">{it.value}</TableCell>
              <TableCell>{it.sort}</TableCell>
              <TableCell>
                <Badge variant={it.status ? "default" : "secondary"}>
                  {it.status ? t("common.enabled") : t("common.disabled")}
                </Badge>
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-2">
                  <Can code="dict:update">
                    <DictItemDialog mode="edit" dictId={dictId} item={it} />
                  </Can>
                  <Can code="dict:delete">
                    <DeleteDictItemDialog item={it} dictId={dictId} />
                  </Can>
                </div>
              </TableCell>
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  );
}

function DictDialog({ mode, dict, onDone }: { mode: "create" | "edit"; dict?: Dict; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const createMut = useMutation(createDict);
  const updateMut = useMutation(updateDict);

  const [type, setType] = useState(dict?.type ?? "");
  const [name, setName] = useState(dict?.name ?? "");
  const [description, setDescription] = useState(dict?.description ?? "");
  const [status, setStatus] = useState(dict?.status ?? true);

  const submit = async () => {
    try {
      if (mode === "create") {
        await createMut.mutateAsync({ type, name, description, status });
        toast.success(t("dictPage.createdToast"));
      } else if (dict) {
        await updateMut.mutateAsync({ id: dict.id, name, description, status });
        toast.success(t("dictPage.updatedToast"));
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
            {t("dictPage.addDict")}
          </Button>
        ) : (
          <Button variant="ghost" size="sm">
            {t("common.edit")}
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{mode === "create" ? t("dictPage.addDict") : t("common.edit")}</DialogTitle>
          <DialogDescription>{t("dictPage.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="type">{t("common.type")}</Label>
            <Input id="type" value={type} disabled={mode === "edit"} onChange={(e) => setType(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="name">{t("common.name")}</Label>
            <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="desc">{t("common.description")}</Label>
            <Input id="desc" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="status">{t("common.status")}</Label>
            <Switch id="status" checked={status} onCheckedChange={setStatus} />
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

function DeleteDictDialog({ dict, onDone }: { dict: Dict; onDone: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const mut = useMutation(deleteDict);

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: dict.id });
      toast.success(t("dictPage.deletedToast"));
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
          <DialogDescription>{dict.type}</DialogDescription>
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

function DictItemDialog({ mode, dictId, item }: { mode: "create" | "edit"; dictId: bigint; item?: DictItem }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const createMut = useMutation(createDictItem);
  const updateMut = useMutation(updateDictItem);

  const [label, setLabel] = useState(item?.label ?? "");
  const [value, setValue] = useState(item?.value ?? "");
  const [sort, setSort] = useState(item?.sort ?? 0);
  const [status, setStatus] = useState(item?.status ?? true);

  const invalidate = () =>
    qc.invalidateQueries({ queryKey: createConnectQueryKey({ schema: listDictItems, cardinality: "finite" }) });

  const submit = async () => {
    try {
      if (mode === "create") {
        await createMut.mutateAsync({ dictId, label, value, sort, status });
        toast.success(t("dictPage.itemCreatedToast"));
      } else if (item) {
        await updateMut.mutateAsync({ id: item.id, label, value, sort, status });
        toast.success(t("dictPage.itemUpdatedToast"));
      }
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {mode === "create" ? (
          <Button size="sm">
            <PlusIcon className="size-4" />
            {t("dictPage.addItem")}
          </Button>
        ) : (
          <Button variant="ghost" size="sm">
            {t("common.edit")}
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{mode === "create" ? t("dictPage.addItem") : t("common.edit")}</DialogTitle>
          <DialogDescription>{t("dictPage.items")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="label">{t("common.label")}</Label>
            <Input id="label" value={label} onChange={(e) => setLabel(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="value">{t("common.value")}</Label>
            <Input id="value" value={value} onChange={(e) => setValue(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="sort">{t("common.sort")}</Label>
            <Input
              id="sort"
              type="number"
              value={sort}
              onChange={(e) => setSort(Number(e.target.value))}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="istatus">{t("common.status")}</Label>
            <Switch id="istatus" checked={status} onCheckedChange={setStatus} />
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

function DeleteDictItemDialog({ item, dictId }: { item: DictItem; dictId: bigint }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const mut = useMutation(deleteDictItem);
  void dictId;

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: item.id });
      toast.success(t("dictPage.itemDeletedToast"));
      await qc.invalidateQueries({
        queryKey: createConnectQueryKey({ schema: listDictItems, cardinality: "finite" }),
      });
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
          <DialogDescription>{item.label}</DialogDescription>
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
