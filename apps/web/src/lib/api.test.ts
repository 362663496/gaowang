import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiDeleteSession, apiDownload, apiGet, apiPost, readDevSession, writeDevSession } from "./api";

const originalFetch = globalThis.fetch;
const store = new Map<string, string>();
let locationAssign: ReturnType<typeof vi.fn>;
let createObjectURL: ReturnType<typeof vi.fn>;
let revokeObjectURL: ReturnType<typeof vi.fn>;
let appendChild: ReturnType<typeof vi.fn>;
let click: ReturnType<typeof vi.fn>;
let remove: ReturnType<typeof vi.fn>;

beforeEach(() => {
  store.clear();
  locationAssign = vi.fn();
  createObjectURL = vi.fn(() => "blob:inventory");
  revokeObjectURL = vi.fn();
  click = vi.fn();
  remove = vi.fn();
  appendChild = vi.fn();
  const localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
  };
  vi.stubGlobal("window", { localStorage, dispatchEvent: vi.fn(), location: { pathname: "/dashboard", assign: locationAssign } });
  vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
  vi.stubGlobal("document", {
    createElement: vi.fn(() => ({
      href: "",
      download: "",
      rel: "",
      click,
      remove,
    })),
    body: { appendChild },
  });
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

  it("downloads authenticated blobs and revokes the object URL", async () => {
    writeDevSession({ userId: "00000000-0000-0000-0000-000000000001", role: "staff" });
    const blob = new Blob(["xlsx-bytes"], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" });
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(blob, {
        status: 200,
        headers: {
          "Content-Type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
          "Content-Disposition": 'attachment; filename="inventory-2026-07-17.xlsx"',
        },
      }),
    );
    globalThis.fetch = fetchMock;

    await apiDownload("/inventory/export?low_stock=true", "fallback.xlsx");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/inventory/export?low_stock=true",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
        headers: expect.objectContaining({
          "X-Dev-Role": "staff",
          "X-Dev-User-ID": "00000000-0000-0000-0000-000000000001",
        }),
      }),
    );
    expect(createObjectURL).toHaveBeenCalled();
    expect(appendChild).toHaveBeenCalled();
    expect(click).toHaveBeenCalled();
    expect(remove).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:inventory");
    const anchor = (document.createElement as ReturnType<typeof vi.fn>).mock.results[0]?.value as { download: string };
    expect(anchor.download).toBe("inventory-2026-07-17.xlsx");
  });

  it("surfaces structured errors when download fails", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "INTERNAL", message: "export failed" } }), { status: 500 }),
    );

    await expect(apiDownload("/inventory/export", "fallback.xlsx")).rejects.toMatchObject({
      code: "INTERNAL",
      message: "export failed",
      status: 500,
    });
    expect(createObjectURL).not.toHaveBeenCalled();
  });
});
