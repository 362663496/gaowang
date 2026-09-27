import { afterEach, describe, expect, it, vi } from "vitest";
import { copyText, createApiToken, getApiToken, mcpConfigJson, mcpEndpoint, revokeApiToken } from "./api-token";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("api token helpers", () => {
  it("loads the current token metadata", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ token: { prefix: "gw_abcd", created_at: "2026-09-14T00:00:00Z", last_used_at: null } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(getApiToken()).resolves.toEqual({
      token: { prefix: "gw_abcd", created_at: "2026-09-14T00:00:00Z", last_used_at: null },
    });
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/v1/auth/api-token", expect.objectContaining({ method: "GET" }));
  });

  it("creates a token and builds remote MCP config", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ token: { prefix: "gw_abcd", created_at: "2026-09-14T00:00:00Z", last_used_at: null }, secret: "gw_secret" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const created = await createApiToken();
    expect(created.secret).toBe("gw_secret");
    expect(mcpEndpoint("https://stock.example")).toBe("https://stock.example/api/v1/mcp");
    expect(mcpConfigJson("https://stock.example", created.secret)).toContain("Bearer gw_secret");
    expect(mcpConfigJson("https://stock.example", created.secret)).toContain("https://stock.example/api/v1/mcp");
  });

  it("revokes the token with DELETE", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    await revokeApiToken();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/v1/auth/api-token", expect.objectContaining({ method: "DELETE" }));
  });

  it("copies via execCommand when the page is not a secure context", async () => {
    const textarea = {
      value: "",
      style: {} as CSSStyleDeclaration,
      setAttribute: vi.fn(),
      focus: vi.fn(),
      select: vi.fn(),
      setSelectionRange: vi.fn(),
      remove: vi.fn(),
    };
    const execCommand = vi.fn().mockReturnValue(true);
    const appendChild = vi.fn();
    vi.stubGlobal("window", { isSecureContext: false });
    vi.stubGlobal("navigator", {});
    vi.stubGlobal("document", {
      createElement: vi.fn(() => textarea),
      execCommand,
      body: { appendChild },
    });

    await copyText("gw_secret");

    expect(textarea.value).toBe("gw_secret");
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(textarea.remove).toHaveBeenCalled();
  });
});
