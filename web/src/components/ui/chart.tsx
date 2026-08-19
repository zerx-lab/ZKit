// Minimal shadcn-style chart wrapper over recharts.
// Import recharts primitives from here for consistent styling.

import {
  Bar,
  BarChart,
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { cn } from "@/lib/utils";
import type { ComponentProps } from "react";

export {
  Bar,
  BarChart,
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
};

// ChartContainer: a responsive wrapper with a consistent min-height.
export function ChartContainer({
  className,
  children,
  height = 260,
}: {
  className?: string;
  children: React.ReactNode;
  height?: number;
}) {
  return (
    <div className={cn("w-full", className)}>
      <ResponsiveContainer width="100%" height={height}>
        {children as React.ReactElement}
      </ResponsiveContainer>
    </div>
  );
}

// Semantic chart colors. These reference the project's chart palette / theme
// tokens, which are hex values (not HSL channels) — so they must NOT be wrapped
// in hsl(). Wrapping a hex value in hsl() yields an invalid color that browsers
// render as black.
export const CHART_COLORS = {
  primary: "var(--chart-1)",
  secondary: "var(--chart-3)",
  success: "var(--chart-2)",
  danger: "#ef4444",
  muted: "var(--muted-foreground)",
} as const;

// Tooltip content with shadcn card styling.
export function ChartTooltipContent(
  props: ComponentProps<typeof Tooltip>,
) {
  return (
    <Tooltip
      contentStyle={{
        background: "var(--card)",
        border: "1px solid var(--border)",
        borderRadius: "0.5rem",
        fontSize: "0.75rem",
      }}
      {...props}
    />
  );
}
