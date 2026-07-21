"use client";

import { Result } from "antd";
import { useSession } from "@/components/layout/session-context";
import { anyPermission, hasPermission } from "@/lib/permissions";

export function PermissionGate({
  permission,
  anyOf,
  children,
  fallback,
}: {
  permission?: string;
  anyOf?: string[];
  children: React.ReactNode;
  fallback?: React.ReactNode;
}) {
  const { permissions, loading } = useSession();
  if (loading) return null;
  const allowed = permission
    ? hasPermission(permissions, permission)
    : anyOf
      ? anyPermission(permissions, anyOf)
      : true;
  if (!allowed) {
    return fallback ?? <Result status="403" subTitle="当前账号没有访问此页面的权限。" title="无权限" />;
  }
  return <>{children}</>;
}
