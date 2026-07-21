"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { SessionUser } from "@/lib/api";
import {
  fetchAuthMe,
  permissionsRefreshEvent,
  sessionExpiredEvent,
} from "@/lib/api";
import { hasPermission as checkPermission } from "@/lib/permissions";

type SessionState = {
  user: SessionUser | null;
  permissions: string[];
  loading: boolean;
  error: string;
  hasPermission: (key: string) => boolean;
  refresh: (options?: { silent?: boolean }) => Promise<void>;
  clear: () => void;
};

const SessionContext = createContext<SessionState | null>(null);

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<SessionUser | null>(null);
  const [permissions, setPermissions] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const clear = useCallback(() => {
    setUser(null);
    setPermissions([]);
  }, []);

  const refresh = useCallback(async (options?: { silent?: boolean }) => {
    const silent = options?.silent ?? false;
    if (!silent) {
      setLoading(true);
    }
    setError("");
    try {
      const data = await fetchAuthMe();
      setUser({
        id: data.user.id,
        name: data.user.name,
        email: data.user.email,
        role: data.user.role,
      });
      setPermissions(data.permissions ?? []);
    } catch (err) {
      clear();
      setError(err instanceof Error ? err.message : "会话加载失败");
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, [clear]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const onFocus = () => {
      void refresh({ silent: true });
    };
    const onPermissions = () => {
      void refresh({ silent: true });
    };
    const onExpired = () => {
      clear();
    };
    window.addEventListener("focus", onFocus);
    window.addEventListener(permissionsRefreshEvent, onPermissions);
    window.addEventListener(sessionExpiredEvent, onExpired);
    return () => {
      window.removeEventListener("focus", onFocus);
      window.removeEventListener(permissionsRefreshEvent, onPermissions);
      window.removeEventListener(sessionExpiredEvent, onExpired);
    };
  }, [clear, refresh]);

  const value = useMemo<SessionState>(
    () => ({
      user,
      permissions,
      loading,
      error,
      hasPermission: (key: string) => checkPermission(permissions, key),
      refresh,
      clear,
    }),
    [user, permissions, loading, error, refresh, clear],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionState {
  const ctx = useContext(SessionContext);
  if (!ctx) {
    throw new Error("useSession must be used within SessionProvider");
  }
  return ctx;
}
