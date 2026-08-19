import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import type { SysParam } from "@/gen/zerx/v1/param_pb";
import {
  createParam,
  deleteParam,
  listParams,
  updateParam,
} from "@/gen/zerx/v1/param-SysParamService_connectquery";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/params")({ component: ParamsPage });

function useInvalidate() {
  const qc = useQueryClient();
  return () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listParams, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function ParamsPage() {
  const { t } = useI18n();
  const { data, isPending } = useQuery(listParams, { page: { page: 1, pageSize: 100 }, keyword: "" });
  const params = data?.params ?? [];

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("paramPage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("paramPage.subtitle")}</p>
        </div>
        <Can code="param:create">
          <ParamDialog mode="create" />
        </Can>
      </div>

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden py-0">
        <Table>
          <TableHeader><TableRow>
            <TableHead>{t("common.key")}</TableHead>
            <TableHead>{t("common.name")}</TableHead>
            <TableHead>{t("common.value")}</TableHead>
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
            ) : params.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                  {t("common.noData")}
                </TableCell>
              </TableRow>
            ) : (
              params.map((p) => (
                <TableRow key={String(p.id)}>
                  <TableCell className="font-mono text-xs">{p.key}</TableCell>
                  <TableCell>{p.name}</TableCell>
                  <TableCell className="max-w-xs truncate">{p.value}</TableCell>
                  <TableCell className="max-w-xs truncate text-muted-foreground">{p.description}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Can code="param:update">
                        <ParamDialog mode="edit" param={p} />
                      </Can>
                      <Can code="param:delete">
                        <DeleteParamDialog param={p} />
                      </Can>
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

function ParamDialog({ mode, param }: { mode: "create" | "edit"; param?: SysParam }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidate();
  const createMut = useMutation(createParam);
  const updateMut = useMutation(updateParam);

  const [key, setKey] = useState(param?.key ?? "");
  const [name, setName] = useState(param?.name ?? "");
  const [value, setValue] = useState(param?.value ?? "");
  const [description, setDescription] = useState(param?.description ?? "");

  const submit = async () => {
    try {
      if (mode === "create") {
        await createMut.mutateAsync({ key, name, value, description });
        toast.success(t("paramPage.createdToast"));
      } else if (param) {
        await updateMut.mutateAsync({ id: param.id, name, value, description });
        toast.success(t("paramPage.updatedToast"));
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
          <Button>
            <PlusIcon className="size-4" />
            {t("paramPage.addParam")}
          </Button>
        ) : (
          <Button variant="ghost" size="sm">
            {t("common.edit")}
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{mode === "create" ? t("paramPage.addParam") : t("common.edit")}</DialogTitle>
          <DialogDescription>{t("paramPage.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="key">{t("common.key")}</Label>
            <Input id="key" value={key} disabled={mode === "edit"} onChange={(e) => setKey(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="name">{t("common.name")}</Label>
            <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="value">{t("common.value")}</Label>
            <Textarea id="value" value={value} onChange={(e) => setValue(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="desc">{t("common.description")}</Label>
            <Input id="desc" value={description} onChange={(e) => setDescription(e.target.value)} />
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

function DeleteParamDialog({ param }: { param: SysParam }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidate();
  const mut = useMutation(deleteParam);

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: param.id });
      toast.success(t("paramPage.deletedToast"));
      await invalidate();
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
          <DialogDescription>{param.key}</DialogDescription>
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
