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
