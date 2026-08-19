import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "@tanstack/react-form";
import { createFileRoute } from "@tanstack/react-router";
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { PlayIcon, PlusIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Can } from "@/components/can";
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
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Job, JobExecution } from "@/gen/zerx/v1/job_pb";
import {
  createJob,
  deleteJob,
  listHandlers,
  listJobExecutions,
  listJobs,
  runJobNow,
  updateJob,
} from "@/gen/zerx/v1/job-JobService_connectquery";
import { firstErrorMessage } from "@/lib/form";
import { useI18n, type TranslateFn } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/jobs")({ component: JobsPage });

const PAGE_SIZE = 10;
const EXEC_PAGE_SIZE = 20;

function useInvalidateJobs() {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listJobs, cardinality: "finite" }),
    });
}

const columnHelper = createColumnHelper<Job>();

function JobsPage() {
  const { t } = useI18n();
  const [page, setPage] = useState(1);
  const [historyJob, setHistoryJob] = useState<Job | null>(null);

  const { data, isPending } = useQuery(listJobs, {
    page: { page, pageSize: PAGE_SIZE },
  });

  const jobs = data?.jobs ?? [];
  const total = data ? Number(data.total) : 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const columns = useMemo(
    () => [
      columnHelper.accessor("name", { header: t("jobs.nameLabel") }),
      columnHelper.accessor("handler", { header: t("jobs.handlerLabel") }),
      columnHelper.accessor("cronExpr", { header: t("jobs.cronLabel") }),
      columnHelper.accessor("enabled", {
        header: t("jobs.enabledLabel"),
        cell: (info) => (
          <Badge variant={info.getValue() ? "default" : "secondary"}>
            {info.getValue() ? t("common.enabled") : t("common.disabled")}
          </Badge>
        ),
      }),
      columnHelper.accessor("lastRunAt", {
        header: t("jobs.lastRunAt"),
        cell: (info) =>
          info.getValue() ? new Date(info.getValue()).toLocaleString() : "—",
      }),
      columnHelper.display({
        id: "actions",
        header: () => <span className="sr-only">{t("common.actions")}</span>,
        cell: (info) => (
          <JobRowActions
            job={info.row.original}
            onHistory={() => setHistoryJob(info.row.original)}
          />
        ),
      }),
    ],
    [t],
  );

  const table = useReactTable({ data: jobs, columns, getCoreRowModel: getCoreRowModel() });

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("jobs.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("jobs.subtitle")}</p>
        </div>
        <Can code="job:create">
          <CreateJobDialog />
        </Can>
      </div>

      <Card className="flex min-h-0 flex-1 flex-col gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>{table.getHeaderGroups().map((hg) => (
            <TableRow key={hg.id}>
              {hg.headers.map((h) => (
                <TableHead key={h.id}>
                  {h.isPlaceholder ? null : flexRender(h.column.columnDef.header, h.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}</TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : table.getRowModel().rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  {t("jobs.noJobs")}
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between gap-4 border-t bg-card px-4 py-3"><p className="text-sm text-muted-foreground">{t("common.total", { count: total })}</p>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            {t("common.previous")}
          </Button>
          <span className="text-sm tabular-nums">{t("common.pageOf", { page, pages: pageCount })}</span>
          <Button variant="outline" size="sm" disabled={page >= pageCount} onClick={() => setPage((p) => p + 1)}>
            {t("common.next")}
          </Button>
        </div></div>
      </Card>

      {historyJob && (
        <JobHistoryDrawer job={historyJob} onClose={() => setHistoryJob(null)} />
      )}
    </div>
  );
}

function createJobSchema(t: TranslateFn) {
  return z.object({
    name: z.string().min(1, t("validation.nameRequired")),
    handler: z.string().min(1, t("validation.required")),
    cronExpr: z.string().min(1, t("validation.required")),
    enabled: z.boolean(),
    description: z.string(),
  });
}

function JobFormFields({
  form,
  handlers,
}: {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  form: any;
  handlers: { key: string; description: string }[];
}) {
  const { t } = useI18n();
  return (
    <>
      <form.Field name="name">
        {(field: { name: string; state: { value: string; meta: { errors: unknown[] } }; handleBlur: () => void; handleChange: (v: string) => void }) => {
          const error = firstErrorMessage(field.state.meta.errors);
          return (
            <div className="flex flex-col gap-2">
              <Label htmlFor={field.name}>{t("jobs.nameLabel")}</Label>
              <Input
                id={field.name}
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
              />
              {error && <p className="text-destructive text-sm">{error}</p>}
            </div>
          );
        }}
      </form.Field>

      <form.Field name="handler">
        {(field: { name: string; state: { value: string; meta: { errors: unknown[] } }; handleBlur: () => void; handleChange: (v: string) => void }) => {
          const error = firstErrorMessage(field.state.meta.errors);
          return (
            <div className="flex flex-col gap-2">
              <Label htmlFor={field.name}>{t("jobs.handlerLabel")}</Label>
              <Select value={field.state.value} onValueChange={field.handleChange}>
                <SelectTrigger id={field.name} onBlur={field.handleBlur}>
                  <SelectValue placeholder={t("jobs.handlerPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {handlers.map((h) => (
                    <SelectItem key={h.key} value={h.key}>
                      {h.key} — {h.description}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {error && <p className="text-destructive text-sm">{error}</p>}
            </div>
          );
        }}
      </form.Field>

      <form.Field name="cronExpr">
        {(field: { name: string; state: { value: string; meta: { errors: unknown[] } }; handleBlur: () => void; handleChange: (v: string) => void }) => {
          const error = firstErrorMessage(field.state.meta.errors);
          return (
            <div className="flex flex-col gap-2">
              <Label htmlFor={field.name}>{t("jobs.cronLabel")}</Label>
              <Input
                id={field.name}
                placeholder={t("jobs.cronPlaceholder")}
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
              />
              {error && <p className="text-destructive text-sm">{error}</p>}
            </div>
          );
        }}
      </form.Field>

      <form.Field name="description">
        {(field: { name: string; state: { value: string; meta: { errors: unknown[] } }; handleBlur: () => void; handleChange: (v: string) => void }) => (
          <div className="flex flex-col gap-2">
            <Label htmlFor={field.name}>{t("jobs.descriptionLabel")}</Label>
            <Input
              id={field.name}
              value={field.state.value}
              onBlur={field.handleBlur}
              onChange={(e) => field.handleChange(e.target.value)}
            />
          </div>
        )}
      </form.Field>

      <form.Field name="enabled">
        {(field: { name: string; state: { value: boolean }; handleChange: (v: boolean) => void }) => (
          <div className="flex items-center justify-between">
            <Label htmlFor={field.name}>{t("jobs.enabledLabel")}</Label>
            <Switch
              id={field.name}
              checked={field.state.value}
              onCheckedChange={(v) => field.handleChange(v)}
            />
          </div>
        )}
      </form.Field>
    </>
  );
}

function CreateJobDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidateJobs();
  const mutation = useMutation(createJob);
  const { data: handlersData } = useQuery(listHandlers, {});
  const handlers = handlersData?.handlers ?? [];

  const form = useForm({
    defaultValues: { name: "", handler: "", cronExpr: "", enabled: true, description: "" },
    validators: { onChange: createJobSchema(t) },
    onSubmit: async ({ value }) => {
      try {
        await mutation.mutateAsync(value);
        toast.success(t("jobs.createdToast"));
        await invalidate();
        form.reset();
        setOpen(false);
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <PlusIcon className="size-4" />
          {t("jobs.add")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("jobs.addTitle")}</DialogTitle>
        </DialogHeader>
        <form className="flex flex-col gap-4" onSubmit={(e) => { e.preventDefault(); void form.handleSubmit(); }}>
          <JobFormFields form={form} handlers={handlers} />
          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>{t("common.create")}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function JobRowActions({ job, onHistory }: { job: Job; onHistory: () => void }) {
  const { t } = useI18n();
  const runMutation = useMutation(runJobNow);
  const invalidate = useInvalidateJobs();

  const handleRun = async () => {
    try {
      await runMutation.mutateAsync({ id: job.id });
      toast.success(t("jobs.runToast"));
      await invalidate();
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  return (
    <div className="flex justify-end gap-1">
      <Can code="job:run">
        <Button variant="ghost" size="sm" onClick={() => void handleRun()} disabled={runMutation.isPending}>
          <PlayIcon className="size-3.5" />
          {t("jobs.runBtn")}
        </Button>
      </Can>
      <Button variant="ghost" size="sm" onClick={onHistory}>
        {t("jobs.history")}
      </Button>
      <Can code="job:update">
        <EditJobDialog job={job} />
      </Can>
      <Can code="job:delete">
        <DeleteJobDialog job={job} />
      </Can>
    </div>
  );
}

function EditJobDialog({ job }: { job: Job }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidateJobs();
  const mutation = useMutation(updateJob);
  const { data: handlersData } = useQuery(listHandlers, {});
  const handlers = handlersData?.handlers ?? [];

  const form = useForm({
    defaultValues: {
      name: job.name,
      handler: job.handler,
      cronExpr: job.cronExpr,
      enabled: job.enabled,
      description: job.description,
    },
    validators: { onChange: createJobSchema(t) },
    onSubmit: async ({ value }) => {
      try {
        await mutation.mutateAsync({ id: job.id, ...value });
        toast.success(t("jobs.updatedToast"));
        await invalidate();
        setOpen(false);
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">{t("common.edit")}</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("jobs.editTitle")}</DialogTitle>
          <DialogDescription>{job.name}</DialogDescription>
        </DialogHeader>
        <form className="flex flex-col gap-4" onSubmit={(e) => { e.preventDefault(); void form.handleSubmit(); }}>
          <JobFormFields form={form} handlers={handlers} />
          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>{t("common.save")}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function DeleteJobDialog({ job }: { job: Job }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidateJobs();
  const mutation = useMutation(deleteJob);

  const handleDelete = async () => {
    try {
      await mutation.mutateAsync({ id: job.id });
      toast.success(t("jobs.deletedToast"));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
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
          <DialogTitle>{t("jobs.deleteTitle")}</DialogTitle>
          <DialogDescription>{t("jobs.deleteDesc", { name: job.name })}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>{t("common.cancel")}</Button>
          <Button variant="destructive" disabled={mutation.isPending} onClick={() => void handleDelete()}>
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const execColHelper = createColumnHelper<JobExecution>();

function JobHistoryDrawer({ job, onClose }: { job: Job; onClose: () => void }) {
  const { t } = useI18n();
  const [page, setPage] = useState(1);
  const { data, isPending } = useQuery(listJobExecutions, {
    jobId: job.id,
    page: { page, pageSize: EXEC_PAGE_SIZE },
  });
  const execs = data?.executions ?? [];
  const total = data ? Number(data.total) : 0;
  const pageCount = Math.max(1, Math.ceil(total / EXEC_PAGE_SIZE));

  const columns = useMemo(
    () => [
      execColHelper.accessor("startedAt", {
        header: t("jobs.execStarted"),
        cell: (info) => (info.getValue() ? new Date(info.getValue()).toLocaleString() : "—"),
      }),
      execColHelper.accessor("status", {
        header: t("jobs.execStatus"),
        cell: (info) => (
          <Badge variant={info.getValue() === "ok" ? "default" : "destructive"}>
            {info.getValue()}
          </Badge>
        ),
      }),
      execColHelper.accessor("durationMs", {
        header: t("jobs.execDuration"),
        cell: (info) => Number(info.getValue()),
      }),
      execColHelper.accessor("error", {
        header: t("jobs.execError"),
        cell: (info) =>
          info.getValue() ? (
            <span className="text-xs text-destructive">{info.getValue()}</span>
          ) : null,
      }),
    ],
    [t],
  );

  const table = useReactTable({ data: execs, columns, getCoreRowModel: getCoreRowModel() });

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("jobs.historyTitle", { name: job.name })}</DialogTitle>
        </DialogHeader>
        <div className="flex max-h-[60vh] min-h-0 flex-col overflow-hidden">
          <Table>
            <TableHeader>{table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id}>
                {hg.headers.map((h) => (
                  <TableHead key={h.id}>
                    {h.isPlaceholder ? null : flexRender(h.column.columnDef.header, h.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}</TableHeader>
            <TableBody>
              {isPending ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-20 text-center text-muted-foreground">{t("common.loading")}</TableCell>
                </TableRow>
              ) : execs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-20 text-center text-muted-foreground">{t("jobs.noHistory")}</TableCell>
                </TableRow>
              ) : (
                table.getRowModel().rows.map((row) => (
                  <TableRow key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <TableCell key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>
                    ))}
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
          <div className="mt-3 flex shrink-0 items-center justify-between px-2 text-sm text-muted-foreground">
            <span>{t("common.total", { count: total })}</span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>{t("common.previous")}</Button>
              <span>{t("common.pageOf", { page, pages: pageCount })}</span>
              <Button variant="outline" size="sm" disabled={page >= pageCount} onClick={() => setPage((p) => p + 1)}>{t("common.next")}</Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
