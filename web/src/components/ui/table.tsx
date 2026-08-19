"use client"

import * as React from "react"

import { cn } from "@/lib/utils"

const tableClassName = "w-full table-fixed caption-bottom text-sm"

function Table({ className, children, ...props }: React.ComponentProps<"table">) {
  const headers: React.ReactElement[] = []
  const bodies: React.ReactElement[] = []
  const extras: React.ReactNode[] = []

  React.Children.forEach(children, (child) => {
    if (!React.isValidElement(child)) {
      extras.push(child)
      return
    }
    if (child.type === TableHeader) {
      headers.push(child)
      return
    }
    if (child.type === TableBody) {
      bodies.push(child)
      return
    }
    extras.push(child)
  })

  return (
    <div
      data-slot="table-container"
      className="flex min-h-0 w-full flex-1 flex-col overflow-hidden"
    >
      <div data-slot="table-head-wrap" className="w-full shrink-0 bg-table-header">
        <table
          data-slot="table-head-table"
          className={cn(tableClassName, className)}
          {...props}
        >
          {headers}
        </table>
      </div>
      <div
        data-slot="table-body-wrap"
        className="min-h-0 flex-1 overflow-auto"
      >
        <table
          data-slot="table-body-table"
          className={cn(tableClassName, className)}
          {...props}
        >
          {bodies}
        </table>
      </div>
      {extras}
    </div>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("[&_tr]:border-b", className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&_tr:last-child]:border-0", className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t bg-card font-medium [&>tr]:last:border-b-0",
        className
      )}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-accent/55 has-[td[colspan]]:bg-card has-[td[colspan]]:hover:bg-card has-aria-expanded:bg-accent/60 data-[state=selected]:bg-accent/70",
        className
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "h-10 bg-table-header px-4 text-left align-middle text-xs font-semibold whitespace-nowrap text-muted-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "px-4 py-3 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className
      )}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
}
