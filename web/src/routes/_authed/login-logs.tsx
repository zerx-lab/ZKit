import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { DownloadIcon, Trash2Icon } from "lucide-react";
import { type FormEvent, useMemo, useState } from "react";
import { toast } from "sonner";

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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cleanLogs, listLoginLogs } from "@/gen/zerx/v1/log-LogService_connectquery";
import { type LoginLog, LogType } from "@/gen/zerx/v1/log_pb";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { authedFetch } from "@/lib/transport";

export const Route = createFileRoute("/_authed/login-logs")({ component: LoginLogsPage });

const PAGE_SIZE = 10;

function useInvalidate() {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listLoginLogs, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function toStartRFC3339(date: string) {
  if (!date) return "";
  return new Date(date + "T00:00:00").toISOString();
}

function toEndRFC3339(date: string) {
  if (!date) return "";
  return new Date(date + "T23:59:59").toISOString();
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

const columnHelper = createColumnHelper<LoginLog>();

function LoginLogsPage() {
  const { t } = useI18n();
  const { roles } = usePermissions();
  const [page, setPage] = useState(1);
  const [keywordInput, setKeywordInput] = useState("");
  const [keyword, setKeyword] = useState("");
  const [startDate, setStartDate] = useState("");
  const [endDate, setEndDate] = useState("");
  const [successFilter, setSuccessFilter] = useState("0");

  const queryInput = {
    page: { page, pageSize: PAGE_SIZE },
    keyword,
    startAt: toStartRFC3339(startDate),
    endAt: toEndRFC3339(endDate),
    success: parseInt(successFilter, 10),
  };

  const { data, isPending } = useQuery(listLoginLogs, queryInput);

  const logs = data?.logs ?? [];
  const total = data ? Number(data.total) : 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const columns = useMemo(
    () => [
      columnHelper.accessor("id", {
        header: t("common.id"),
        cell: (info) => <span className="font-mono text-xs">{String(info.getValue())}</span>,
      }),
      columnHelper.accessor("email", { header: t("common.email") }),
      columnHelper.accessor("ip", { header: t("logPage.ip") }),
      columnHelper.accessor("userAgent", {
        header: "User-Agent",
        cell: (info) => (
          <span
            className="block max-w-[200px] truncate text-xs text-muted-foreground"
            title={info.getValue()}
          >
            {info.getValue()}
          </span>
        ),
      }),
      columnHelper.accessor("success", {
        header: t("logPage.status"),
        cell: (info) => (
          <Badge variant={info.getValue() ? "default" : "destructive"}>
            {info.getValue() ? t("logPage.success") : t("logPage.failure")}
          </Badge>
        ),
      }),
      columnHelper.accessor("error", {
        header: t("logPage.error"),
        cell: (info) =>
          info.getValue() ? (
            <span className="block max-w-xs truncate text-xs text-muted-foreground">
              {info.getValue()}
            </span>
          ) : null,
      }),
      columnHelper.accessor("createdAt", {
        header: t("common.created"),
        cell: (info) => new Date(info.getValue()).toLocaleString(),
      }),
    ],
    [t],
  );

  const table = useReactTable({ data: logs, columns, getCoreRowModel: getCoreRowModel() });

  const applySearch = (e: FormEvent) => {
    e.preventDefault();
    setPage(1);
    setKeyword(keywordInput.trim());
  };

  const handleExport = async () => {
    toast.info(t("logPage.exportToast"));
    const params = new URLSearchParams();
    if (keyword) params.set("keyword", keyword);
    if (startDate) params.set("start_at", toStartRFC3339(startDate));
    if (endDate) params.set("end_at", toEndRFC3339(endDate));
    if (successFilter !== "0") params.set("success", successFilter);
    const res = await authedFetch(`/api/export/login-logs?${params.toString()}`);
    if (!res.ok) { toast.error("Export failed"); return; }
    const blob = await res.blob();
    downloadBlob(blob, "login-logs.xlsx");
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("logPage.loginTitle")}</h1>
          <p className="text-sm text-muted-foreground">{t("logPage.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Can code="login-log:export">
            <Button variant="outline" size="sm" onClick={() => void handleExport()}>
              <DownloadIcon className="size-4" />
              {t("common.export")}
            </Button>
          </Can>
          {roles.includes("admin") && <CleanLogsDialog />}
        </div>
      </div>

      {/* Filters */}
      <div className="flex shrink-0 flex-wrap gap-2 rounded-lg border bg-card p-3"><form className="flex items-center gap-2" onSubmit={applySearch}>
        <Input
          placeholder={t("logPage.searchPlaceholder")}
          value={keywordInput}
          onChange={(e) => setKeywordInput(e.target.value)}
          className="max-w-xs"
        />
        <Button type="submit" variant="secondary" size="sm">
          {t("common.search")}
        </Button>
      </form>
      <Select
        value={successFilter}
        onValueChange={(v) => { setSuccessFilter(v); setPage(1); }}
      >
        <SelectTrigger className="w-32">
          <SelectValue placeholder={t("logPage.allResults")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="0">{t("logPage.allResults")}</SelectItem>
          <SelectItem value="1">{t("logPage.success")}</SelectItem>
          <SelectItem value="2">{t("logPage.failure")}</SelectItem>
        </SelectContent>
      </Select>
      <div className="flex items-center gap-1">
        <Label className="text-sm text-muted-foreground whitespace-nowrap">{t("logPage.filterStartDate")}</Label>
        <Input
          type="date"
          value={startDate}
          onChange={(e) => { setStartDate(e.target.value); setPage(1); }}
          className="w-36"
        />
      </div>
      <div className="flex items-center gap-1">
        <Label className="text-sm text-muted-foreground whitespace-nowrap">{t("logPage.filterEndDate")}</Label>
        <Input
          type="date"
          value={endDate}
          onChange={(e) => { setEndDate(e.target.value); setPage(1); }}
          className="w-36"
        />
      </div></div>

      <Card className="flex min-h-0 flex-1 flex-col gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>{table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder
                    ? null
                    : flexRender(header.column.columnDef.header, header.getContext())}
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
                  {t("common.noData")}
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
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t("common.previous")}
          </Button>
          <span className="text-sm tabular-nums">
            {t("common.pageOf", { page, pages: pageCount })}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= pageCount}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("common.next")}
          </Button>
        </div></div>
      </Card>
    </div>
  );
}

function CleanLogsDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [days, setDays] = useState(30);
  const invalidate = useInvalidate();
  const mut = useMutation(cleanLogs);

  const handleClean = async () => {
    try {
      const res = await mut.mutateAsync({ type: LogType.LOGIN, days });
      toast.success(t("logPage.cleanedToast", { count: Number(res.removed) }));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">
          <Trash2Icon className="size-4" />
          {t("logPage.clean")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("logPage.clean")}</DialogTitle>
          <DialogDescription>{t("logPage.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <Label htmlFor="days-login">{t("logPage.cleanDays")}</Label>
          <Input
            id="days-login"
            type="number"
            min={1}
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" disabled={mut.isPending} onClick={() => void handleClean()}>
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
