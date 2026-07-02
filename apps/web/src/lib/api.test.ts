import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiDeleteSession, apiGet, apiPost, readDevSession, writeDevSession } from "./api";

const originalFetch = globalThis.fetch;
const store = new Map<string, string>();
let locationAssign: ReturnType<typeof vi.fn>;

beforeEach(() => {
  store.clear();
  locationAssign = vi.fn();
  const localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
  };
  vi.stubGlobal("window", { localStorage, dispatchEvent: vi.fn(), location: { pathname: "/dashboard", assign: locationAssign } });
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("api client", () => {
  it("sends stored development auth headers", async () => {
    writeDevSession({ userId: "00000000-0000-0000-0000-000000000001", role: "admin" });
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [] }), { status: 200 }));
    globalThis.fetch = fetchMock;

    await apiGet<{ items: unknown[] }>("/products");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/products",
      expect.objectContaining({
        headers: expect.objectContaining({
          "X-Dev-Role": "admin",
          "X-Dev-User-ID": "00000000-0000-0000-0000-000000000001",
        }),
      }),
    );
  });

  it("throws structured API errors", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "VALIDATION", message: "bad input" } }), { status: 400 }),
    );

    await expect(apiPost("/shops", { name: "" })).rejects.toMatchObject({
      code: "VALIDATION",
      message: "bad input",
      status: 400,
    });
  });

  it("clears the session and redirects to login when auth expires", async () => {
    writeDevSession({ userId: "00000000-0000-0000-0000-000000000001", role: "admin" });
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "UNAUTHORIZED", message: "login required" } }), { status: 401 }),
    );

    await expect(apiGet("/users")).rejects.toMatchObject({ status: 401 });

    expect(readDevSession()).toEqual({ userId: "", role: "admin" });
    expect(locationAssign).toHaveBeenCalledWith("/login");
  });

  it("stores and clears the development session", () => {
    writeDevSession({ userId: "00000000-0000-0000-0000-000000000002", role: "staff" });
    expect(readDevSession()).toEqual({ userId: "00000000-0000-0000-0000-000000000002", role: "staff" });

    apiDeleteSession();

    expect(readDevSession()).toEqual({ userId: "", role: "admin" });
  });

  it("exposes an error class for UI state", () => {
    const error = new ApiError("NOPE", "failed", 500);
    expect(error).toBeInstanceOf(Error);
    expect(error.message).toBe("failed");
  });
});
