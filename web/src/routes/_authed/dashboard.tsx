import { useQuery } from "@connectrpc/connect-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  ActivityIcon,
  CalendarDaysIcon,
  CheckCircle2Icon,
  HashIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  TriangleAlertIcon,
  UserRoundIcon,
  UsersIcon,
} from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Bar,
  BarChart,
  CartesianGrid,
  CHART_COLORS,
  ChartContainer,
  ChartTooltipContent,
  ComposedChart,
  Line,
  XAxis,
  YAxis,
} from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { me } from "@/gen/zerx/v1/auth-AuthService_connectquery";
import { getDashboardStats } from "@/gen/zerx/v1/dashboard-DashboardService_connectquery";
import type {
  GetDashboardStatsResponse,
  TimePoint,
} from "@/gen/zerx/v1/dashboard_pb";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/_authed/dashboard")({
  component: DashboardPage,
});

type SeriesKey = "userGrowth" | "loginSuccess" | "loginFailure" | "operations";

type SeriesPoint = Record<SeriesKey, number> & { date: string };

function mergeTimeSeries(stats: GetDashboardStatsResponse): SeriesPoint[] {
  const values = new Map<string, SeriesPoint>();
  const merge = (points: TimePoint[], key: SeriesKey) => {
    for (const point of points) {
      const current = values.get(point.date) ?? {
        date: point.date,
        userGrowth: 0,
        loginSuccess: 0,
        loginFailure: 0,
        operations: 0,
      };
      current[key] = Number(point.value);
      values.set(point.date, current);
    }
  };

  merge(stats.userGrowth, "userGrowth");
  merge(stats.loginSuccess, "loginSuccess");
  merge(stats.loginFailure, "loginFailure");
  merge(stats.operationCount, "operations");
  return [...values.values()].sort((left, right) => left.date.localeCompare(right.date));
}

function sumPoints(points: TimePoint[]): number {
  return points.reduce((sum, point) => sum + Number(point.value), 0);
}

function Metric({
  icon: Icon,
  label,
  value,
  caption,
  positive = false,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: ReactNode;
  caption: string;
  positive?: boolean;
}) {
  return (
    <div className="min-w-0 border-b border-border p-5 last:border-b-0 sm:[&:nth-child(odd)]:border-r lg:border-b-0 lg:border-r lg:last:border-r-0">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Icon className="size-4" />
        <span>{label}</span>
      </div>
      <div className="mt-3 flex items-baseline gap-3">
        <div className="text-2xl font-semibold tracking-tight tabular-nums">{value}</div>
      </div>
      <p className={cn("mt-2 text-xs text-muted-foreground", positive && "text-chart-2")}>
        {caption}
      </p>
    </div>
  );
}

function SummaryRow({
  icon: Icon,
  label,
  detail,
  value,
  tone = "default",
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  detail: string;
  value: ReactNode;
  tone?: "default" | "success" | "warning";
}) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 border-b border-border py-3.5 last:border-b-0">
      <span className="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Icon className="size-4" />
      </span>
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{label}</p>
        <p className="mt-0.5 truncate text-xs text-muted-foreground">{detail}</p>
      </div>
      <span
        className={cn(
          "text-sm font-semibold tabular-nums",
          tone === "success" && "text-chart-2",
          tone === "warning" && "text-destructive",
        )}
      >
        {value}
      </span>
    </div>
  );
}

function PanelHeader({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-14 items-center gap-4 border-b border-border px-5 py-3">
      <div className="min-w-0">
        <h2 className="truncate text-sm font-semibold">{title}</h2>
        <p className="mt-0.5 truncate text-xs text-muted-foreground">{description}</p>
      </div>
      {action ? <div className="ml-auto shrink-0">{action}</div> : null}
    </div>
  );
}

function DashboardPage() {
  const { locale, t } = useI18n();
  const { data: meData, isPending: mePending } = useQuery(me);
  const {
    data: stats,
    error: statsError,
    isPending: statsPending,
    isFetching,
    refetch,
  } = useQuery(getDashboardStats, {});

  const user = meData?.user;
  const series = stats ? mergeTimeSeries(stats) : [];
  const recentUsers = stats ? sumPoints(stats.userGrowth) : 0;
  const successfulLogins = stats ? sumPoints(stats.loginSuccess) : 0;
  const failedLogins = stats ? sumPoints(stats.loginFailure) : 0;
  const operationCount = stats ? sumPoints(stats.operationCount) : 0;
  const todaySeries = series.at(-1);
  const dateLabel = new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(new Date());

  const metricValue = (value: bigint | undefined) => {
    if (statsError) return "—";
    if (statsPending || value === undefined) return <Skeleton className="h-7 w-10" />;
    return Number(value);
  };

  return (
    <div className="mx-auto flex h-full min-h-0 w-full max-w-[1600px] flex-col gap-4 overflow-auto">
      <div className="flex flex-col gap-4 pb-1 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <div className="mb-1.5 flex items-center gap-2 text-xs text-muted-foreground">
            <CalendarDaysIcon className="size-3.5" />
            <span>{dateLabel}</span>
            <span aria-hidden="true">·</span>
            <span>{t("dashboard.period14")}</span>
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">{t("dashboard.overviewTitle")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t("dashboard.overviewDesc")}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={isFetching}
          onClick={() => void refetch()}
        >
          <RefreshCwIcon className={cn("size-4", isFetching && "animate-spin")} />
          {t("common.refresh")}
        </Button>
      </div>

      <section className="grid overflow-hidden rounded-lg border border-border bg-card sm:grid-cols-2 lg:grid-cols-4">
        <Metric
          icon={UsersIcon}
          label={t("dashboard.totalUsers")}
          value={metricValue(stats?.totalUsers)}
          caption={t("dashboard.recentUsers", { count: recentUsers })}
          positive={recentUsers > 0}
        />
        <Metric
          icon={ShieldCheckIcon}
          label={t("dashboard.totalRoles")}
          value={metricValue(stats?.totalRoles)}
          caption={t("dashboard.roleCaption")}
        />
        <Metric
          icon={ActivityIcon}
          label={t("dashboard.activeSessions")}
          value={metricValue(stats?.activeSessions)}
          caption={t("dashboard.sessionCaption")}
        />
        <Metric
          icon={HashIcon}
          label={t("dashboard.todayLogins")}
          value={metricValue(stats?.todayLogins)}
          caption={t("dashboard.todayLoginSummary", {
            success: todaySeries?.loginSuccess ?? 0,
            failure: todaySeries?.loginFailure ?? 0,
          })}
          positive={(todaySeries?.loginSuccess ?? 0) > 0 && (todaySeries?.loginFailure ?? 0) === 0}
        />
      </section>

      {statsError ? (
        <div className="rounded-lg border border-border bg-card px-5 py-10 text-center text-sm text-muted-foreground">
          {t("dashboard.noPermission")}
        </div>
      ) : (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(19rem,1fr)]">
          <section className="min-w-0 rounded-lg border border-border bg-card">
            <PanelHeader
              title={t("dashboard.accessTrend")}
              description={t("dashboard.accessTrendDesc")}
              action={
                <div className="hidden items-center gap-3 text-[11px] text-muted-foreground sm:flex">
                  <span className="flex items-center gap-1.5"><i className="size-1.5 rounded-full bg-chart-1" />{t("dashboard.newUsers")}</span>
                  <span className="flex items-center gap-1.5"><i className="size-1.5 rounded-full bg-chart-2" />{t("dashboard.loginSuccess")}</span>
                  <span className="flex items-center gap-1.5"><i className="size-1.5 rounded-full bg-destructive" />{t("dashboard.loginFailure")}</span>
                </div>
              }
            />
            <div className="p-4 sm:p-5">
              {statsPending ? (
                <Skeleton className="h-[260px] w-full" />
              ) : series.length === 0 ? (
                <div className="flex h-[260px] items-center justify-center text-sm text-muted-foreground">{t("common.noData")}</div>
              ) : (
                <ChartContainer height={260}>
                  <ComposedChart data={series} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
                    <CartesianGrid vertical={false} strokeDasharray="2 4" className="stroke-border" />
                    <XAxis dataKey="date" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} minTickGap={24} />
                    <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} />
                    <ChartTooltipContent cursor={{ fill: "var(--muted)", fillOpacity: 0.35 }} />
                    <Bar dataKey="loginSuccess" name={t("dashboard.loginSuccess")} stackId="logins" fill={CHART_COLORS.success} radius={[2, 2, 0, 0]} maxBarSize={18} />
                    <Bar dataKey="loginFailure" name={t("dashboard.loginFailure")} stackId="logins" fill={CHART_COLORS.danger} radius={[2, 2, 0, 0]} maxBarSize={18} />
                    <Line type="linear" dataKey="userGrowth" name={t("dashboard.newUsers")} stroke={CHART_COLORS.primary} dot={false} activeDot={{ r: 3 }} strokeWidth={2} />
                  </ComposedChart>
                </ChartContainer>
              )}
            </div>
          </section>

          <section className="min-w-0 rounded-lg border border-border bg-card">
            <PanelHeader title={t("dashboard.activitySummary")} description={t("dashboard.activitySummaryDesc")} />
            <div className="px-5 py-1">
              <SummaryRow icon={CheckCircle2Icon} label={t("dashboard.successfulLogins")} detail={t("dashboard.period14")} value={statsPending ? <Skeleton className="h-5 w-8" /> : successfulLogins} tone="success" />
              <SummaryRow icon={TriangleAlertIcon} label={t("dashboard.failedLogins")} detail={t("dashboard.period14")} value={statsPending ? <Skeleton className="h-5 w-8" /> : failedLogins} tone={failedLogins > 0 ? "warning" : "default"} />
              <SummaryRow icon={ActivityIcon} label={t("dashboard.operations")} detail={t("dashboard.period14")} value={statsPending ? <Skeleton className="h-5 w-8" /> : operationCount} />
              <SummaryRow icon={UserRoundIcon} label={t("dashboard.activeSessions")} detail={t("dashboard.currentSnapshot")} value={metricValue(stats?.activeSessions)} />
            </div>
          </section>
        </div>
      )}

      <div className="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(19rem,1fr)]">
        <section className="min-w-0 rounded-lg border border-border bg-card">
          <PanelHeader title={t("dashboard.operationTrend")}
            description={t("dashboard.operationTrendDesc")} />
          <div className="p-4 sm:p-5">
            {statsError ? (
              <div className="flex h-[210px] items-center justify-center text-sm text-muted-foreground">{t("dashboard.noPermission")}</div>
            ) : statsPending ? (
              <Skeleton className="h-[210px] w-full" />
            ) : series.length === 0 ? (
              <div className="flex h-[210px] items-center justify-center text-sm text-muted-foreground">{t("common.noData")}</div>
            ) : (
              <ChartContainer height={210}>
                <BarChart data={series} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
                  <CartesianGrid vertical={false} strokeDasharray="2 4" className="stroke-border" />
                  <XAxis dataKey="date" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} minTickGap={24} />
                  <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} allowDecimals={false} />
                  <ChartTooltipContent />
                  <Bar dataKey="operations" name={t("dashboard.operations")} fill={CHART_COLORS.primary} radius={[2, 2, 0, 0]} maxBarSize={28} />
                </BarChart>
              </ChartContainer>
            )}
          </div>
        </section>

        <section className="min-w-0 rounded-lg border border-border bg-card">
          <PanelHeader title={t("dashboard.currentUser")} description={t("dashboard.accountSummaryDesc")} />
          <div className="p-5">
            {mePending || !user ? (
              <Skeleton className="h-40 w-full" />
            ) : (
              <>
                <div className="flex items-center gap-3 border-b border-border pb-4">
                  <span className="flex size-10 items-center justify-center rounded-md bg-primary/10 text-sm font-semibold text-primary">
                    {(user.name || user.email || "?").charAt(0).toUpperCase()}
                  </span>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold">{user.name || user.email}</p>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">{user.email}</p>
                  </div>
                </div>
                <dl className="mt-4 grid grid-cols-2 gap-x-5 gap-y-4 text-xs">
                  <div>
                    <dt className="text-muted-foreground">{t("dashboard.accountId")}</dt>
                    <dd className="mt-1 font-medium tabular-nums">{String(user.id)}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">{t("common.status")}</dt>
                    <dd className="mt-1 font-medium">{user.status ? t("common.enabled") : t("common.disabled")}</dd>
                  </div>
                  <div className="col-span-2">
                    <dt className="text-muted-foreground">{t("common.roles")}</dt>
                    <dd className="mt-1.5 flex flex-wrap gap-1.5">
                      {user.roles.length > 0 ? user.roles.map((role) => (
                        <Badge key={role} variant="outline" className="rounded px-1.5 py-0 font-medium">
                          {t(`roles.${role}`) !== `roles.${role}` ? t(`roles.${role}`) : role}
                        </Badge>
                      )) : <span className="text-muted-foreground">—</span>}
                    </dd>
                  </div>
                </dl>
              </>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
