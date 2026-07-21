export type Role = "admin" | "staff";

export type SessionUser = {
  id: string;
  name: string;
  email: string;
  role: Role;
};

export type AuthPayload = {
  user: SessionUser;
  permissions: string[];
};

const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api/v1";
export const sessionExpiredEvent = "gaowang:session-expired";
export const permissionsRefreshEvent = "gaowang:permissions-refresh";

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export async function apiGet<T>(path: string): Promise<T> {
  return request<T>(path, { method: "GET" });
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    headers: body instanceof FormData ? undefined : { "Content-Type": "application/json" },
    body: body instanceof FormData ? body : JSON.stringify(body ?? {}),
  });
}

export async function apiDownload(path: string, fallbackFilename: string): Promise<void> {
  const response = await fetch(`${baseUrl}${path}`, {
    method: "GET",
    credentials: "include",
  });
  if (!response.ok) {
    await throwResponseError(response);
  }
  const blob = await response.blob();
  const filename = filenameFromContentDisposition(response.headers.get("Content-Disposition")) ?? fallbackFilename;
  const objectUrl = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement("a");
    anchor.href = objectUrl;
    anchor.download = filename;
    anchor.rel = "noopener";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    URL.revokeObjectURL(objectUrl);
  }
}

export async function apiPut<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });
}

export async function request<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...init.headers,
    },
  });
  if (!response.ok) {
    await throwResponseError(response);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

function filenameFromContentDisposition(header: string | null): string | undefined {
  if (!header) {
    return undefined;
  }
  const utfMatch = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (utfMatch?.[1]) {
    try {
      return decodeURIComponent(utfMatch[1].trim());
    } catch {
      return utfMatch[1].trim();
    }
  }
  const plainMatch = /filename="?([^";]+)"?/i.exec(header);
  return plainMatch?.[1]?.trim() || undefined;
}

export async function fetchAuthMe(): Promise<AuthPayload> {
  return apiGet<AuthPayload>("/auth/me");
}

export async function login(loginValue: string, password: string): Promise<AuthPayload> {
  return apiPost<AuthPayload>("/auth/login", { login: loginValue, password });
}

export async function logout(): Promise<void> {
  await apiPost<void>("/auth/logout", {});
}

function readError(response: Response): Promise<ApiError> {
  return response
    .json()
    .then((data: { error?: { code?: string; message?: string } }) => {
      return new ApiError(
        data.error?.code ?? "REQUEST_FAILED",
        data.error?.message ?? `请求失败：${response.status}`,
        response.status,
      );
    })
    .catch(() => new ApiError("REQUEST_FAILED", `请求失败：${response.status}`, response.status));
}

async function throwResponseError(response: Response): Promise<never> {
  const error = await readError(response);
  if (error.status === 401) {
    notifySessionExpired();
    redirectToLogin();
  } else if (error.status === 403) {
    notifyPermissionsRefresh();
  }
  throw error;
}

function notifySessionExpired(): void {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(sessionExpiredEvent));
  }
}

function notifyPermissionsRefresh(): void {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(permissionsRefreshEvent));
  }
}

function redirectToLogin(): void {
  if (typeof window !== "undefined" && window.location.pathname !== "/login") {
    window.location.assign("/login");
  }
}
