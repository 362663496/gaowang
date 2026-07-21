"use client";

import { Alert, App, Button, Checkbox, Flex, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { useSession } from "@/components/layout/session-context";
import { apiGet, apiPut } from "@/lib/api";
import {
  grantWithDependencies,
  hasPermission,
  revokeWithDependents,
  type PermissionCatalogItem,
} from "@/lib/permissions";

type PermissionsResponse = {
  catalog: PermissionCatalogItem[];
  staff_permissions: string[];
};

type MatrixRow = PermissionCatalogItem & { key: string };

export default function PermissionsPage() {
  const { message } = App.useApp();
  const { hasPermission: can } = useSession();
  const [catalog, setCatalog] = useState<PermissionCatalogItem[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiGet<PermissionsResponse>("/permissions");
      setCatalog(data.catalog);
      setSelected(data.staff_permissions ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载权限失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const rows: MatrixRow[] = useMemo(() => catalog.map((item) => ({ ...item, key: item.key })), [catalog]);

  function toggleStaff(key: string, checked: boolean) {
    setSelected((current) => (checked ? grantWithDependencies(current, key, catalog) : revokeWithDependents(current, key, catalog)));
  }

  async function save() {
    setSaving(true);
    setError("");
    try {
      const data = await apiPut<PermissionsResponse>("/permissions", { permissions: selected });
      setCatalog(data.catalog);
      setSelected(data.staff_permissions ?? []);
      message.success("员工权限已保存");
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存权限失败");
    } finally {
      setSaving(false);
    }
  }

  const columns: TableProps<MatrixRow>["columns"] = [
    {
      title: "模块",
      dataIndex: "module_label",
      width: 120,
      onCell: (record, index) => {
        const firstIndex = rows.findIndex((row) => row.module === record.module);
        if (index !== firstIndex) return { rowSpan: 0 };
        return { rowSpan: rows.filter((row) => row.module === record.module).length };
      },
    },
    { title: "操作", dataIndex: "action_label", width: 140 },
    {
      title: "权限键",
      dataIndex: "key",
      width: 220,
      render: (value: string) => <span className="mono">{value}</span>,
    },
    {
      title: "管理员",
      width: 100,
      render: () => <Checkbox checked disabled aria-label="管理员权限（锁定）" />,
    },
    {
      title: "员工",
      width: 160,
      render: (_, row) => {
        if (!row.staff_assignable) {
          return (
            <Flex gap={8} align="center">
              <Checkbox checked={false} disabled aria-label={`${row.key} 管理员专属`} />
              <Tag>管理员专属</Tag>
            </Flex>
          );
        }
        const checked = hasPermission(selected, row.key);
        const requires = row.requires ?? [];
        const required = requires.length > 0 ? `依赖：${requires.join("、")}` : undefined;
        return (
          <Checkbox
            aria-label={`员工权限 ${row.key}`}
            checked={checked}
            disabled={!can("permission.update")}
            onChange={(event) => toggleStaff(row.key, event.target.checked)}
          >
            {required ? <span className="muted">{required}</span> : null}
          </Checkbox>
        );
      },
    },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader
        actions={
          can("permission.update") ? (
            <Button loading={saving} type="primary" onClick={() => void save()}>
              保存员工权限
            </Button>
          ) : null
        }
        description="配置所有 staff 账号共享的业务权限。管理员永远拥有全部权限。"
        title="权限管理"
      />
      {error ? <Alert message={error} showIcon type="error" /> : null}
      <Table<MatrixRow>
        columns={columns}
        dataSource={rows}
        loading={loading}
        pagination={false}
        rowKey="key"
        scroll={{ x: 900 }}
        size="middle"
      />
    </Flex>
  );
}
