import type { FormEvent } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { submitCreateUser } from "./create-user";

const originalFetch = globalThis.fetch;
const OriginalFormData = globalThis.FormData;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.stubGlobal("FormData", OriginalFormData);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("submitCreateUser", () => {
  it("resets the submitted form after the async request completes", async () => {
    let currentTargetCleared = false;
    const reset = vi.fn();
    const preventDefault = vi.fn();
    const formElement = { reset } as unknown as HTMLFormElement;
    const event = {
      preventDefault,
      get currentTarget() {
        return currentTargetCleared ? null : formElement;
      },
    } as unknown as FormEvent<HTMLFormElement>;

    class FakeFormData {
      get(key: string): FormDataEntryValue | null {
        const values: Record<string, string> = {
          name: "Smoke User",
          email: "smoke@example.com",
          password: "password123",
          role: "staff",
        };
        return values[key] ?? null;
      }
    }
    vi.stubGlobal("FormData", FakeFormData);
    globalThis.fetch = vi.fn(async (_url, init) => {
      currentTargetCleared = true;
      expect(JSON.parse(String(init?.body))).toEqual({
        name: "Smoke User",
        email: "smoke@example.com",
        password: "password123",
        role: "staff",
      });
      return new Response(JSON.stringify({ item: { id: "user-1", name: "Smoke User", email: "smoke@example.com", role: "staff" } }), {
        status: 200,
      });
    });

    const user = await submitCreateUser(event);

    expect(preventDefault).toHaveBeenCalledOnce();
    expect(reset).toHaveBeenCalledOnce();
    expect(user.id).toBe("user-1");
  });
});
