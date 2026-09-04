import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AuthService } from "@/gen/zerx/v1/auth_pb";
import { UserService } from "@/gen/zerx/v1/user_pb";
import { getAccessToken, getRefreshToken, getSessionId, setTokens } from "@/lib/auth";
import { authedFetch, transport } from "@/lib/transport";

const REFRESH_URL = `/api/${AuthService.typeName}/Refresh`;
const LIST_USERS_URL = `/api/${UserService.typeName}/ListUsers`;
const UPLOAD_URL = "/api/upload";

// One observed fetch call, captured at call time. connect-web reuses (and
// mutates) the same Headers instance across a retry, so the mock's recorded
// arguments cannot be inspected after the fact.
interface Seen {
  url: string;
  bearer: string | null;
  method: string | undefined;
}

type Route = (call: Seen) => Response;

interface FetchHarness {
  seen: Seen[];
  to(url: string): Seen[];
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function unauthenticated(): Response {
  return json(401, { code: "unauthenticated", message: "token expired" });
}

function refreshed(): Response {
  return json(200, { accessToken: "new-access" });
}

function mockFetch(route: Route): FetchHarness {
  const seen: Seen[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
      const call: Seen = { url, bearer: new Headers(init?.headers).get("Authorization"), method: init?.method };
      seen.push(call);
      return route(call);
    }),
  );
  return { seen, to: (url) => seen.filter((c) => c.url === url) };
}

// window.location is unforgeable in jsdom; vitest exposes it as a configurable
// global so it can be swapped for a plain object.
function stubLocation(pathname = "/users"): { pathname: string; href: string } {
  const loc = { pathname, href: `http://localhost${pathname}` };
  vi.stubGlobal("location", loc);
  return loc;
}

describe("transport auth interceptor", () => {
  beforeEach(() => {
    setTokens("old-access", "refresh-1", "sess-1");
  });

  it("shares one refresh across concurrent 401s and retries each request once", async () => {
    const loc = stubLocation();
    const fetchMock = mockFetch((call) => {
      if (call.url === REFRESH_URL) return refreshed();
      // ListUsers: reject the stale token, accept the refreshed one.
      return call.bearer === "Bearer new-access" ? json(200, { users: [] }) : unauthenticated();
    });

    const client = createClient(UserService, transport);
    const [a, b] = await Promise.all([client.listUsers({}), client.listUsers({})]);

    expect(a.users).toEqual([]);
    expect(b.users).toEqual([]);

    const refreshCalls = fetchMock.to(REFRESH_URL);
    expect(refreshCalls).toHaveLength(1);
    // The refresh request goes through the bare transport: no bearer token.
    expect(refreshCalls[0]?.bearer).toBeNull();

    // 2 initial failures + 2 retries, and the retries use the new token.
    const bearers = fetchMock.to(LIST_USERS_URL).map((c) => c.bearer);
    expect(bearers).toHaveLength(4);
    expect(bearers.filter((b) => b === "Bearer old-access")).toHaveLength(2);
    expect(bearers.filter((b) => b === "Bearer new-access")).toHaveLength(2);

    expect(getAccessToken()).toBe("new-access");
    // Refresh keeps the existing refresh token and session id.
    expect(getRefreshToken()).toBe("refresh-1");
    expect(getSessionId()).toBe("sess-1");
    expect(loc.href).toBe("http://localhost/users");
  });

  it("clears tokens and redirects to /login when the refresh fails", async () => {
    const loc = stubLocation();
    const fetchMock = mockFetch(() => unauthenticated());

    const client = createClient(UserService, transport);
    const err = await client.listUsers({}).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.Unauthenticated);

    expect(fetchMock.to(REFRESH_URL)).toHaveLength(1);
    // No retry after a failed refresh.
    expect(fetchMock.to(LIST_USERS_URL)).toHaveLength(1);

    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
    expect(getSessionId()).toBeNull();
    expect(loc.href).toBe("/login");
  });

  it("does not redirect again when already on /login", async () => {
    const loc = stubLocation("/login");
    mockFetch(() => unauthenticated());

    const client = createClient(UserService, transport);
    await expect(client.listUsers({})).rejects.toBeInstanceOf(ConnectError);

    expect(getAccessToken()).toBeNull();
    expect(loc.href).toBe("http://localhost/login");
  });

  it("rethrows non-401 errors without refreshing", async () => {
    const loc = stubLocation();
    const fetchMock = mockFetch(() => json(403, { code: "permission_denied", message: "nope" }));

    const client = createClient(UserService, transport);
    const err = await client.listUsers({}).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ConnectError);
    expect((err as ConnectError).code).toBe(Code.PermissionDenied);

    expect(fetchMock.to(REFRESH_URL)).toHaveLength(0);
    expect(fetchMock.seen).toHaveLength(1);
    expect(getAccessToken()).toBe("old-access");
    expect(loc.href).toBe("http://localhost/users");
  });

  it("never refreshes for AuthService calls themselves", async () => {
    stubLocation();
    const fetchMock = mockFetch(() => unauthenticated());

    const client = createClient(AuthService, transport);
    const err = await client.me({}).catch((e: unknown) => e);

    expect((err as ConnectError).code).toBe(Code.Unauthenticated);
    expect(fetchMock.to(REFRESH_URL)).toHaveLength(0);
    expect(fetchMock.seen).toHaveLength(1);
    // Tokens are untouched: a failing auth call surfaces as-is.
    expect(getAccessToken()).toBe("old-access");
  });

  it("does not refresh when there is no refresh token", async () => {
    localStorage.removeItem("zerx.refreshToken");
    const loc = stubLocation();
    const fetchMock = mockFetch(() => unauthenticated());

    const client = createClient(UserService, transport);
    await expect(client.listUsers({})).rejects.toBeInstanceOf(ConnectError);

    expect(fetchMock.to(REFRESH_URL)).toHaveLength(0);
    expect(getAccessToken()).toBeNull();
    expect(loc.href).toBe("/login");
  });
});

describe("authedFetch", () => {
  beforeEach(() => {
    setTokens("old-access", "refresh-1");
  });

  it("refreshes once on 401 and retries with the new token", async () => {
    stubLocation();
    const fetchMock = mockFetch((call) => {
      if (call.url === REFRESH_URL) return refreshed();
      return call.bearer === "Bearer new-access"
        ? new Response("ok", { status: 200 })
        : new Response(null, { status: 401 });
    });

    const res = await authedFetch(UPLOAD_URL, { method: "POST" });

    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ok");
    expect(fetchMock.to(REFRESH_URL)).toHaveLength(1);
    const uploads = fetchMock.to(UPLOAD_URL);
    expect(uploads.map((c) => c.bearer)).toEqual(["Bearer old-access", "Bearer new-access"]);
    expect(uploads.map((c) => c.method)).toEqual(["POST", "POST"]);
    expect(getAccessToken()).toBe("new-access");
  });

  it("shares the in-flight refresh with the connect transport", async () => {
    stubLocation();
    const fetchMock = mockFetch((call) => {
      if (call.url === REFRESH_URL) return refreshed();
      if (call.bearer === "Bearer new-access") {
        return call.url === LIST_USERS_URL ? json(200, { users: [] }) : new Response("ok", { status: 200 });
      }
      return call.url === LIST_USERS_URL ? unauthenticated() : new Response(null, { status: 401 });
    });

    const client = createClient(UserService, transport);
    const [list, res] = await Promise.all([client.listUsers({}), authedFetch(UPLOAD_URL)]);

    expect(list.users).toEqual([]);
    expect(res.status).toBe(200);
    expect(fetchMock.to(REFRESH_URL)).toHaveLength(1);
  });

  it("returns the 401 response, clears tokens and redirects when the refresh fails", async () => {
    const loc = stubLocation();
    const fetchMock = mockFetch((call) =>
      call.url === REFRESH_URL ? unauthenticated() : new Response(null, { status: 401 }),
    );

    const res = await authedFetch(UPLOAD_URL);

    expect(res.status).toBe(401);
    expect(fetchMock.to(UPLOAD_URL)).toHaveLength(1);
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
    expect(loc.href).toBe("/login");
  });

  it("passes non-401 responses through untouched", async () => {
    stubLocation();
    const fetchMock = mockFetch(() => new Response("boom", { status: 500 }));

    const res = await authedFetch(UPLOAD_URL);

    expect(res.status).toBe(500);
    expect(fetchMock.seen).toHaveLength(1);
    expect(getAccessToken()).toBe("old-access");
  });
});
