import type { DescMethodUnary } from "@bufbuild/protobuf";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { me } from "@/gen/zerx/v1/auth-AuthService_connectquery";
import { getUserButtons } from "@/gen/zerx/v1/menu-MenuService_connectquery";
import { PermissionProvider, usePermissions } from "@/lib/permissions";

// Minimal shape of the connect-query useQuery result that PermissionProvider reads.
interface FakeQuery {
  data: unknown;
  isPending: boolean;
}

interface Scenario {
  me: FakeQuery;
  buttons: FakeQuery;
}

const { scenario } = vi.hoisted(() => ({ scenario: { current: null as Scenario | null } }));

vi.mock("@connectrpc/connect-query", () => ({
  useQuery: (schema: DescMethodUnary) => {
    const s = scenario.current;
    if (!s) throw new Error("scenario not set");
    if (schema === me) return s.me;
    if (schema === getUserButtons) return s.buttons;
    throw new Error(`unexpected useQuery schema ${schema.parent.typeName}.${schema.name}`);
  },
}));

function loaded(data: unknown): FakeQuery {
  return { data, isPending: false };
}

const PENDING: FakeQuery = { data: undefined, isPending: true };

function Probe({ codes }: { codes: string[] }) {
  const { roles, can, isLoading } = usePermissions();
  return (
    <div>
      <span data-testid="roles">{roles.join(",")}</span>
      <span data-testid="loading">{String(isLoading)}</span>
      {codes.map((code) => (
        <span key={code} data-testid={`can:${code}`}>
          {String(can(code))}
        </span>
      ))}
    </div>
  );
}

function renderWith(s: Scenario, codes: string[]) {
  scenario.current = s;
  render(
    <PermissionProvider>
      <Probe codes={codes} />
    </PermissionProvider>,
  );
}

describe("PermissionProvider", () => {
  it("admin passes every code, even with no granted buttons", () => {
    renderWith(
      { me: loaded({ user: { roles: ["admin"] } }), buttons: loaded({ codes: [] }) },
      ["user:create", "anything:at:all"],
    );

    expect(screen.getByTestId("roles")).toHaveTextContent("admin");
    expect(screen.getByTestId("can:user:create")).toHaveTextContent("true");
    expect(screen.getByTestId("can:anything:at:all")).toHaveTextContent("true");
    expect(screen.getByTestId("loading")).toHaveTextContent("false");
  });

  it("admin among several roles still passes", () => {
    renderWith(
      { me: loaded({ user: { roles: ["viewer", "admin"] } }), buttons: loaded({ codes: [] }) },
      ["user:delete"],
    );

    expect(screen.getByTestId("can:user:delete")).toHaveTextContent("true");
  });

  it("non-admin passes only granted codes", () => {
    renderWith(
      { me: loaded({ user: { roles: ["editor"] } }), buttons: loaded({ codes: ["user:create", "user:update"] }) },
      ["user:create", "user:update", "user:delete"],
    );

    expect(screen.getByTestId("roles")).toHaveTextContent("editor");
    expect(screen.getByTestId("can:user:create")).toHaveTextContent("true");
    expect(screen.getByTestId("can:user:update")).toHaveTextContent("true");
    expect(screen.getByTestId("can:user:delete")).toHaveTextContent("false");
    expect(screen.getByTestId("loading")).toHaveTextContent("false");
  });

  it("denies everything while nothing has loaded and reports isLoading", () => {
    renderWith({ me: PENDING, buttons: PENDING }, ["user:create"]);

    expect(screen.getByTestId("roles")).toHaveTextContent("");
    expect(screen.getByTestId("can:user:create")).toHaveTextContent("false");
    expect(screen.getByTestId("loading")).toHaveTextContent("true");
  });

  it("isLoading is true while only `me` is pending", () => {
    renderWith({ me: PENDING, buttons: loaded({ codes: ["user:create"] }) }, ["user:create"]);

    expect(screen.getByTestId("loading")).toHaveTextContent("true");
    // Granted codes already apply even before roles arrive.
    expect(screen.getByTestId("can:user:create")).toHaveTextContent("true");
  });

  it("isLoading is true while only `getUserButtons` is pending", () => {
    renderWith({ me: loaded({ user: { roles: ["editor"] } }), buttons: PENDING }, ["user:create"]);

    expect(screen.getByTestId("loading")).toHaveTextContent("true");
    expect(screen.getByTestId("can:user:create")).toHaveTextContent("false");
  });

  it("admin can() does not wait for buttons", () => {
    renderWith({ me: loaded({ user: { roles: ["admin"] } }), buttons: PENDING }, ["user:create"]);

    expect(screen.getByTestId("loading")).toHaveTextContent("true");
    expect(screen.getByTestId("can:user:create")).toHaveTextContent("true");
  });
});

describe("usePermissions", () => {
  it("throws outside a PermissionProvider", () => {
    // Silence React's error boundary noise for the intentional throw.
    vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => render(<Probe codes={[]} />)).toThrow(/within a PermissionProvider/);
  });
});
