import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiDownload, apiGet, apiPost, permissionsRefreshEvent, sessionExpiredEvent } from "./api";

const originalFetch = globalThis.fetch;
let locationAssign: ReturnType<typeof vi.fn>;
let dispatchEvent: ReturnType<typeof vi.fn>;
let createObjectURL: ReturnType<typeof vi.fn>;
let revokeObjectURL: ReturnType<typeof vi.fn>;
let appendChild: ReturnType<typeof vi.fn>;
let click: ReturnType<typeof vi.fn>;
let remove: ReturnType<typeof vi.fn>;

beforeEach(() => {
  locationAssign = vi.fn();
  dispatchEvent = vi.fn();
  createObjectURL = vi.fn(() => "blob:inventory");
  revokeObjectURL = vi.fn();
  click = vi.fn();
  remove = vi.fn();
  appendChild = vi.fn();
  vi.stubGlobal("window", {
    dispatchEvent,
    location: { pathname: "/dashboard", assign: locationAssign },
  });
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
  it("sends credentials and does not attach development headers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [] }), { status: 200 }));
    globalThis.fetch = fetchMock;

    await apiGet<{ items: unknown[] }>("/products");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/products",
      expect.objectContaining({
        credentials: "include",
        headers: expect.not.objectContaining({
          "X-Dev-Role": expect.anything(),
          "X-Dev-User-ID": expect.anything(),
        }),
      }),
    );
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const headers = init.headers as Record<string, string> | undefined;
    expect(headers?.["X-Dev-User-ID"]).toBeUndefined();
    expect(headers?.["X-Dev-Role"]).toBeUndefined();
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

  it("redirects on 401 and keeps session on 403", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "UNAUTHORIZED", message: "login required" } }), { status: 401 }),
    );
    await expect(apiGet("/users")).rejects.toMatchObject({ status: 401 });
    expect(locationAssign).toHaveBeenCalledWith("/login");
    expect(dispatchEvent).toHaveBeenCalledWith(expect.objectContaining({ type: sessionExpiredEvent }));

    locationAssign.mockClear();
    dispatchEvent.mockClear();
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: "FORBIDDEN", message: "permission denied" } }), { status: 403 }),
    );
    await expect(apiGet("/products")).rejects.toMatchObject({ status: 403 });
    expect(locationAssign).not.toHaveBeenCalled();
    expect(dispatchEvent).toHaveBeenCalledWith(expect.objectContaining({ type: permissionsRefreshEvent }));
  });

  it("exposes an error class for UI state", () => {
    const error = new ApiError("NOPE", "failed", 500);
    expect(error).toBeInstanceOf(Error);
    expect(error.message).toBe("failed");
  });

  it("downloads authenticated blobs and revokes the object URL", async () => {
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
      }),
    );
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).headers).toBeUndefined();
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
