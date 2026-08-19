import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { DownloadIcon, Trash2Icon } from "lucide-react";
import { useState } from "react";
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
import { LogType } from "@/gen/zerx/v1/log_pb";
import type { OperationLog } from "@/gen/zerx/v1/log_pb";
import {
  cleanLogs,
  listOperationLogs,
} from "@/gen/zerx/v1/log-LogService_connectquery";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { authedFetch } from "@/lib/transport";

export const Route = createFileRoute("/_authed/operation-logs")({
  component: OperationLogsPage,
});

const PAGE_SIZE = 10;

function useInvalidate() {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listOperationLogs, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

// Convert a YYYY-MM-DD string to start-of-day RFC3339
function toStartRFC3339(date: string) {
  if (!date) return "";
  return new Date(date + "T00:00:00").toISOString();
}

// Convert a YYYY-MM-DD string to end-of-day RFC3339
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

function OperationLogsPage() {
  const { t } = useI18n();
  const { roles } = usePermissions();
  const isAdmin = roles.includes("admin");

  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [methodFilter, setMethodFilter] = useState("");
  const [startDate, setStartDate] = useState("");
  const [endDate, setEndDate] = useState("");
  const [expandedId, setExpandedId] = useState<bigint | null>(null);

  const queryInput = {
    page: { page, pageSize: PAGE_SIZE },
    keyword,
    status: statusFilter,
    method: methodFilter,
    startAt: toStartRFC3339(startDate),
    endAt: toEndRFC3339(endDate),
  };

  const { data, isPending } = useQuery(listOperationLogs, queryInput);

  const logs = data?.logs ?? [];
  const total = data ? Number(data.total) : 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const handleExport = async () => {
    toast.info(t("logPage.exportToast"));
    const params = new URLSearchParams();
    if (keyword) params.set("keyword", keyword);
    if (statusFilter) params.set("status", statusFilter);
    if (methodFilter) params.set("method", methodFilter);
    if (startDate) params.set("start_at", toStartRFC3339(startDate));
    if (endDate) params.set("end_at", toEndRFC3339(endDate));
    const res = await authedFetch(`/api/export/operation-logs?${params.toString()}`);
    if (!res.ok) { toast.error("Export failed"); return; }
    const blob = await res.blob();
    downloadBlob(blob, "operation-logs.xlsx");
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">
            {t("logPage.operationTitle")}
          </h1>
          <p className="text-sm text-muted-foreground">{t("logPage.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Can code="operation-log:export">
            <Button variant="outline" size="sm" onClick={() => void handleExport()}>
              <DownloadIcon className="size-4" />
              {t("common.export")}
            </Button>
          </Can>
          {isAdmin && <CleanDialog />}
        </div>
      </div>

      {/* Filters */}
      <div className="flex shrink-0 flex-wrap gap-2 rounded-lg border bg-card p-3"><Input
        placeholder={t("logPage.searchPlaceholder")}
        value={keyword}
        onChange={(e) => { setKeyword(e.target.value); setPage(1); }}
        className="max-w-xs"
      />
      <Select
        value={statusFilter}
        onValueChange={(v) => { setStatusFilter(v === "_all" ? "" : v); setPage(1); }}
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder={t("logPage.allStatuses")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="_all">{t("logPage.allStatuses")}</SelectItem>
          <SelectItem value="ok">{t("logPage.statusOk")}</SelectItem>
          <SelectItem value="permission_denied">permission_denied</SelectItem>
          <SelectItem value="unauthenticated">unauthenticated</SelectItem>
          <SelectItem value="invalid_argument">invalid_argument</SelectItem>
          <SelectItem value="not_found">not_found</SelectItem>
          <SelectItem value="internal">internal</SelectItem>
        </SelectContent>
      </Select>
      <Input
        placeholder={t("logPage.methodPlaceholder")}
        value={methodFilter}
        onChange={(e) => { setMethodFilter(e.target.value); setPage(1); }}
        className="w-32"
      />
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
          <TableHeader><TableRow>
            <TableHead>{t("common.id")}</TableHead>
            <TableHead>{t("logPage.procedure")}</TableHead>
            <TableHead>Method</TableHead>
            <TableHead>{t("logPage.ip")}</TableHead>
            <TableHead>{t("logPage.latency")}</TableHead>
            <TableHead>{t("logPage.status")}</TableHead>
            <TableHead>User ID</TableHead>
            <TableHead>{t("common.created")}</TableHead>
            <TableHead>{t("logPage.detailTitle")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={9} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : logs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={9} className="h-24 text-center text-muted-foreground">
                  {t("common.noData")}
                </TableCell>
              </TableRow>
            ) : (
              logs.map((log) => (
                <OperationLogRow
                  key={String(log.id)}
                  log={log}
                  expanded={expandedId === log.id}
                  onToggle={() => setExpandedId((prev) => (prev === log.id ? null : log.id))}
                />
              ))
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between gap-4 border-t bg-card px-4 py-3"><p className="text-sm text-muted-foreground">
          {t("common.total", { count: total })}
        </p>
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

function OperationLogRow({
  log,
  expanded,
  onToggle,
}: {
  log: OperationLog;
  expanded: boolean;
  onToggle: () => void;
}) {
  const { t } = useI18n();
  const hasDetail = !!log.detail;

  let prettyDetail = "";
  if (hasDetail) {
    try {
      prettyDetail = JSON.stringify(JSON.parse(log.detail), null, 2);
    } catch {
      prettyDetail = log.detail;
    }
  }

  return (
    <>
      <TableRow className={expanded ? "border-b-0" : undefined}>
        <TableCell className="font-mono text-xs">{String(log.id)}</TableCell>
        <TableCell className="max-w-xs truncate font-mono text-xs">{log.procedure}</TableCell>
        <TableCell>{log.method}</TableCell>
        <TableCell>{log.ip}</TableCell>
        <TableCell>{Number(log.latencyMs)}</TableCell>
        <TableCell>
          <Badge variant={log.status === "ok" ? "default" : "secondary"}>
            {log.status}
          </Badge>
        </TableCell>
        <TableCell>{String(log.userId)}</TableCell>
        <TableCell>
          {log.createdAt ? new Date(log.createdAt).toLocaleString() : "—"}
        </TableCell>
        <TableCell>
          {hasDetail && (
            <Button variant="ghost" size="sm" onClick={onToggle}>
              {expanded ? t("common.cancel") : t("logPage.detailTitle")}
            </Button>
          )}
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow>
          <TableCell colSpan={9} className="bg-muted/50 p-0">
            <pre className="whitespace-pre-wrap break-all px-4 py-3 font-mono text-xs leading-relaxed">
              {prettyDetail}
            </pre>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

function CleanDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [days, setDays] = useState(30);
  const invalidate = useInvalidate();
  const mut = useMutation(cleanLogs);

  const handleClean = async () => {
    try {
      const res = await mut.mutateAsync({ type: LogType.OPERATION, days });
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
          <Label htmlFor="days">{t("logPage.cleanDays")}</Label>
          <Input
            id="days"
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
          <Button
            variant="destructive"
            disabled={mut.isPending}
            onClick={() => void handleClean()}
          >
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
