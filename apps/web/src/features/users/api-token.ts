import { apiDelete, apiGet, apiPut } from "../../lib/api";

export type ApiTokenInfo = {
  prefix: string;
  created_at: string;
  last_used_at: string | null;
};

export type ApiTokenResponse = {
  token: ApiTokenInfo | null;
};

export type ApiTokenSecretResponse = {
  token: ApiTokenInfo;
  secret: string;
};

export async function getApiToken(): Promise<ApiTokenResponse> {
  return apiGet<ApiTokenResponse>("/auth/api-token");
}

export async function createApiToken(): Promise<ApiTokenSecretResponse> {
  return apiPut<ApiTokenSecretResponse>("/auth/api-token", {});
}

export async function revokeApiToken(): Promise<void> {
  await apiDelete<void>("/auth/api-token");
}

export function mcpEndpoint(origin: string): string {
  return `${origin.replace(/\/$/, "")}/api/v1/mcp`;
}

export function mcpConfigJson(origin: string, secret: string): string {
  return `${JSON.stringify(
    {
      mcpServers: {
        gaowang: {
          url: mcpEndpoint(origin),
          headers: { Authorization: `Bearer ${secret}` },
        },
      },
    },
    null,
    2,
  )}\n`;
}

export async function copyText(text: string): Promise<void> {
  if (typeof window !== "undefined" && window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // HTTP sites and some permission prompts fall through to the legacy path.
    }
  }
  copyTextLegacy(text);
}

function copyTextLegacy(text: string): void {
  if (typeof document === "undefined") {
    throw new Error("复制失败，请手动选择文本");
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, text.length);
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) {
    throw new Error("复制失败，请手动选择文本");
  }
}
