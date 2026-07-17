export type Role = "admin" | "staff";

export type DevSession = {
  userId: string;
  role: Role;
};

const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api/v1";
const sessionKey = "gaowang.devSession";
export const devSessionEvent = "gaowang:dev-session";
const emptySession: DevSession = { userId: "", role: "admin" };

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
    headers: {
      ...devHeaders(),
    },
  });
  if (!response.ok) {
    const error = await readError(response);
    if (error.status === 401 || error.status === 403) {
      redirectToLogin();
    }
    throw error;
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

export async function request<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...devHeaders(),
      ...init.headers,
    },
  });
  if (!response.ok) {
    const error = await readError(response);
    if (error.status === 401 || error.status === 403) {
      redirectToLogin();
    }
    throw error;
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

export function readDevSession(): DevSession {
  const storage = getStorage();
  if (!storage) {
    return emptySession;
  }
  const raw = storage.getItem(sessionKey);
  if (!raw) {
    return emptySession;
  }
  try {
    const parsed = JSON.parse(raw) as DevSession;
    return {
      userId: typeof parsed.userId === "string" ? parsed.userId : "",
      role: parsed.role === "staff" ? "staff" : "admin",
    };
  } catch {
    return emptySession;
  }
}

export function writeDevSession(session: DevSession): void {
  getStorage()?.setItem(sessionKey, JSON.stringify(session));
  notifyDevSessionChanged();
}

export function apiDeleteSession(): void {
  getStorage()?.removeItem(sessionKey);
  notifyDevSessionChanged();
}

function devHeaders(): Record<string, string> {
  const session = readDevSession();
  if (!session.userId) {
    return {};
  }
  return {
    "X-Dev-User-ID": session.userId,
    "X-Dev-Role": session.role,
  };
}

async function readError(response: Response): Promise<ApiError> {
  try {
    const data = (await response.json()) as { error?: { code?: string; message?: string } };
    return new ApiError(
      data.error?.code ?? "REQUEST_FAILED",
      data.error?.message ?? `请求失败：${response.status}`,
      response.status,
    );
  } catch {
    return new ApiError("REQUEST_FAILED", `请求失败：${response.status}`, response.status);
  }
}

function getStorage(): Pick<Storage, "getItem" | "setItem" | "removeItem"> | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  return window.localStorage;
}

function notifyDevSessionChanged(): void {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(devSessionEvent));
  }
}

function redirectToLogin(): void {
  apiDeleteSession();
  if (typeof window !== "undefined" && window.location.pathname !== "/login") {
    window.location.assign("/login");
  }
}
