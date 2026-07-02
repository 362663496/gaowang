import type { FormEvent } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { submitChangePassword } from "./password";

const originalFetch = globalThis.fetch;
const OriginalFormData = globalThis.FormData;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.stubGlobal("FormData", OriginalFormData);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("submitChangePassword", () => {
  it("posts the current and new passwords then resets the form", async () => {
    const reset = vi.fn();
    const preventDefault = vi.fn();
    const formElement = { reset } as unknown as HTMLFormElement;
    const event = { preventDefault, currentTarget: formElement } as unknown as FormEvent<HTMLFormElement>;

    class FakeFormData {
      get(key: string): FormDataEntryValue | null {
        const values: Record<string, string> = {
          current_password: "old-password",
          new_password: "new-password",
          confirm_password: "new-password",
        };
        return values[key] ?? null;
      }
    }
    vi.stubGlobal("FormData", FakeFormData);
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));

    await submitChangePassword(event);

    expect(preventDefault).toHaveBeenCalledOnce();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/auth/password",
      expect.objectContaining({
        body: JSON.stringify({ current_password: "old-password", new_password: "new-password" }),
      }),
    );
    expect(reset).toHaveBeenCalledOnce();
  });

  it("rejects mismatched confirmation before sending the request", async () => {
    const event = {
      preventDefault: vi.fn(),
      currentTarget: { reset: vi.fn() },
    } as unknown as FormEvent<HTMLFormElement>;

    class FakeFormData {
      get(key: string): FormDataEntryValue | null {
        const values: Record<string, string> = {
          current_password: "old-password",
          new_password: "new-password",
          confirm_password: "different-password",
        };
        return values[key] ?? null;
      }
    }
    vi.stubGlobal("FormData", FakeFormData);
    globalThis.fetch = vi.fn();

    await expect(submitChangePassword(event)).rejects.toThrow("两次输入的新密码不一致");

    expect(globalThis.fetch).not.toHaveBeenCalled();
  });
});
