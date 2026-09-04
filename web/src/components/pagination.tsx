import { useState, type KeyboardEvent } from "react";
import { ChevronLeftIcon, ChevronRightIcon, MoreHorizontalIcon } from "lucide-react";

import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export const DEFAULT_PAGE_SIZES = [10, 20, 50, 100];

/** Number of pages jumped by an ellipsis button (antd convention). */
const ELLIPSIS_STEP = 5;

type PageItem = number | "prev-gap" | "next-gap";

/**
 * Page number model: all pages when ≤ 7, otherwise first + last always visible with a
 * 5-wide window around the current page and ellipsis gaps.
 */
export function pageItems(page: number, pageCount: number): PageItem[] {
  if (pageCount <= 7) return Array.from({ length: pageCount }, (_, i) => i + 1);
  let start = Math.max(2, page - 2);
  let end = Math.min(pageCount - 1, page + 2);
  if (page <= 4) {
    start = 2;
    end = 6;
  } else if (page >= pageCount - 3) {
    start = pageCount - 5;
    end = pageCount - 1;
  }
  const items: PageItem[] = [1];
  if (start > 2) items.push("prev-gap");
  for (let i = start; i <= end; i++) items.push(i);
  if (end < pageCount - 1) items.push("next-gap");
  items.push(pageCount);
  return items;
}

export interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onChange: (page: number, pageSize: number) => void;
  pageSizeOptions?: number[];
  className?: string;
}

const itemBase =
  "inline-flex h-8 min-w-8 items-center justify-center rounded-md border px-2 text-sm tabular-nums transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-40";
const itemIdle = "border-transparent text-foreground hover:bg-accent hover:text-accent-foreground";
const itemActive = "border-primary bg-card font-semibold text-primary";

export function Pagination({
  page,
  pageSize,
  total,
  onChange,
  pageSizeOptions = DEFAULT_PAGE_SIZES,
  className,
}: PaginationProps) {
  const { t } = useI18n();
  const [jump, setJump] = useState("");

  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const current = Math.min(Math.max(1, page), pageCount);
  const go = (p: number) => {
    const next = Math.min(Math.max(1, p), pageCount);
    if (next !== current) onChange(next, pageSize);
  };

  const commitJump = () => {
    const n = Number.parseInt(jump, 10);
    if (Number.isFinite(n)) go(n);
    setJump("");
  };
  const onJumpKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      commitJump();
    }
  };

  return (
    <div className={cn("flex flex-wrap items-center justify-between gap-x-6 gap-y-2", className)}>
      <p className="text-sm text-muted-foreground">{t("common.total", { count: total })}</p>

      <nav aria-label="pagination" className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <ul className="flex items-center gap-1">
          <li>
            <button
              type="button"
              aria-label={t("common.previous")}
              className={cn(itemBase, itemIdle)}
              disabled={current <= 1}
              onClick={() => go(current - 1)}
            >
              <ChevronLeftIcon className="size-4" />
            </button>
          </li>
          {pageItems(current, pageCount).map((item) =>
            typeof item === "number" ? (
              <li key={item}>
                <button
                  type="button"
                  aria-current={item === current ? "page" : undefined}
                  className={cn(itemBase, item === current ? itemActive : itemIdle)}
                  onClick={() => go(item)}
                >
                  {item}
                </button>
              </li>
            ) : (
              <li key={item}>
                <button
                  type="button"
                  aria-label={t(item === "prev-gap" ? "common.previous" : "common.next")}
                  title={t(item === "prev-gap" ? "common.pagePrevGap" : "common.pageNextGap", {
                    count: ELLIPSIS_STEP,
                  })}
                  className={cn(itemBase, itemIdle, "group text-muted-foreground")}
                  onClick={() => go(item === "prev-gap" ? current - ELLIPSIS_STEP : current + ELLIPSIS_STEP)}
                >
                  <MoreHorizontalIcon className="size-4 group-hover:hidden" />
                  {item === "prev-gap" ? (
                    <span className="hidden text-xs font-medium text-primary group-hover:inline">«</span>
                  ) : (
                    <span className="hidden text-xs font-medium text-primary group-hover:inline">»</span>
                  )}
                </button>
              </li>
            ),
          )}
          <li>
            <button
              type="button"
              aria-label={t("common.next")}
              className={cn(itemBase, itemIdle)}
              disabled={current >= pageCount}
              onClick={() => go(current + 1)}
            >
              <ChevronRightIcon className="size-4" />
            </button>
          </li>
        </ul>

        <Select value={String(pageSize)} onValueChange={(v) => onChange(1, Number(v))}>
          <SelectTrigger size="sm" aria-label={t("common.pageSize")} className="tabular-nums">
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="end">
            {pageSizeOptions.map((size) => (
              <SelectItem key={size} value={String(size)} className="tabular-nums">
                {t("common.perPage", { size })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <label className="flex items-center gap-2 text-sm text-foreground">
          <span>{t("common.jumpTo")}</span>
          <Input
            type="number"
            inputMode="numeric"
            min={1}
            max={pageCount}
            value={jump}
            onChange={(e) => setJump(e.target.value)}
            onKeyDown={onJumpKey}
            onBlur={commitJump}
            aria-label={t("common.jumpTo")}
            className="h-8 w-14 px-2 text-center tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
          />
          <span>{t("common.jumpUnit")}</span>
        </label>
      </nav>
    </div>
  );
}
