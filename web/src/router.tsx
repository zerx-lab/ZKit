import { createRouter } from "@tanstack/react-router";

import { ErrorView } from "@/components/error-view";
import { NotFound } from "@/components/not-found";
import { auth } from "@/lib/auth";
import { queryClient } from "@/lib/query-client";

import { routeTree } from "./routeTree.gen";

export const router = createRouter({
  routeTree,
  context: { queryClient, auth },
  defaultPreload: "intent",
  scrollRestoration: true,
  defaultNotFoundComponent: NotFound,
  defaultErrorComponent: ErrorView,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
