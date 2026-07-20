import { afterEach, describe, expect, it, vi } from "vitest";
import { changePassword } from "./password";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("changePassword", () => {
  it("posts the current and new passwords", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));

    await changePassword({
      current_password: "old-password",
      new_password: "new-password",
      confirm_password: "new-password",
    });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/auth/password",
      expect.objectContaining({
        body: JSON.stringify({ current_password: "old-password", new_password: "new-password" }),
      }),
    );
  });

  it("rejects mismatched confirmation before sending the request", async () => {
    globalThis.fetch = vi.fn();

    await expect(changePassword({
      current_password: "old-password",
      new_password: "new-password",
      confirm_password: "different-password",
    })).rejects.toThrow("两次输入的新密码不一致");

    expect(globalThis.fetch).not.toHaveBeenCalled();
  });
});
