import { afterEach, describe, expect, it, vi } from "vitest";
import { createUser } from "./create-user";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("createUser", () => {
  it("posts the Ant Design form values and returns the user", async () => {
    globalThis.fetch = vi.fn(async (_url, init) => {
      expect(JSON.parse(String(init?.body))).toEqual({
        name: "Smoke User",
        email: "smoke@example.com",
        password: "password123",
        role: "staff",
      });
      return new Response(JSON.stringify({ item: { id: "user-1", name: "Smoke User", email: "smoke@example.com", role: "staff" } }), { status: 200 });
    });

    const user = await createUser({
      name: "Smoke User",
      email: "smoke@example.com",
      password: "password123",
      role: "staff",
    });

    expect(user.id).toBe("user-1");
  });
});
