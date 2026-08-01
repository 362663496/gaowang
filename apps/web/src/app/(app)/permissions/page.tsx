"use client";

import { Alert, App, Button, Checkbox, Empty, Flex, Select, Table, Tag, type TableProps } from "antd";
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

type PermissionUser = {
  id: string;
  name: string;
  email: string;
  permissions: string[];
};

type PermissionsResponse = {
  catalog: PermissionCatalogItem[];
  users: PermissionUser[];
};

type UpdatePermissionsResponse = {
  permissions: string[];
};

type MatrixRow = PermissionCatalogItem & { key: string };

export default function PermissionsPage() {
  const { message } = App.useApp();
  const { hasPermission: can } = useSession();
  const [catalog, setCatalog] = useState<PermissionCatalogItem[]>([]);
  const [users, setUsers] = useState<PermissionUser[]>([]);
  const [selectedUserID, setSelectedUserID] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiGet<PermissionsResponse>("/permissions");
      const nextUsers = data.users ?? [];
      setCatalog(data.catalog);
      setUsers(nextUsers);
      setSelectedUserID((current) => nextUsers.some((user) => user.id === current) ? current : (nextUsers[0]?.id ?? ""));
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
  const selectedUser = users.find((user) => user.id === selectedUserID);
  const selected = selectedUser?.permissions ?? [];

  function toggleUserPermission(key: string, checked: boolean) {
    if (!selectedUser) return;
    const permissions = checked
      ? grantWithDependencies(selected, key, catalog)
      : revokeWithDependents(selected, key, catalog);
    setUsers((current) => current.map((user) => user.id === selectedUser.id ? { ...user, permissions } : user));
  }

  async function save() {
    if (!selectedUser) return;
    setSaving(true);
    setError("");
    try {
      const data = await apiPut<UpdatePermissionsResponse>("/permissions", {
        user_id: selectedUser.id,
        permissions: selected,
      });
      setUsers((current) => current.map((user) => user.id === selectedUser.id ? { ...user, permissions: data.permissions } : user));
      message.success(`${selectedUser.name}的权限已保存`);
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
      title: selectedUser?.name ?? "员工",
      width: 180,
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
            aria-label={`${selectedUser?.name ?? "员工"}权限 ${row.key}`}
            checked={checked}
            disabled={!can("permission.update") || saving}
            onChange={(event) => toggleUserPermission(row.key, event.target.checked)}
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
          can("permission.update") && selectedUser ? (
            <Button loading={saving} type="primary" onClick={() => void save()}>
              保存当前员工权限
            </Button>
          ) : null
        }
        description="选择员工并单独配置业务权限。管理员始终拥有全部权限。"
        title="权限管理"
      />
      {error ? (
        <Alert
          action={<Button size="small" onClick={() => void load()}>重新加载</Button>}
          message={error}
          showIcon
          type="error"
        />
      ) : null}
      {loading ? (
        <Table<MatrixRow> columns={columns} dataSource={rows} loading pagination={false} rowKey="key" scroll={{ x: 900 }} />
      ) : users.length === 0 ? (
        error ? null : <Empty description="暂无员工账号" />
      ) : (
        <Flex gap={16} vertical>
          <Flex align="center" gap={12} wrap>
            <strong>当前员工</strong>
            <Select
              aria-label="选择员工"
              disabled={saving}
              optionFilterProp="label"
              options={users.map((user) => ({ value: user.id, label: `${user.name} · ${user.email}` }))}
              showSearch
              style={{ minWidth: 320 }}
              value={selectedUserID}
              onChange={setSelectedUserID}
            />
            <Tag color="blue">staff</Tag>
          </Flex>
          <Table<MatrixRow>
            columns={columns}
            dataSource={rows}
            pagination={false}
            rowKey="key"
            scroll={{ x: 900 }}
            size="middle"
          />
        </Flex>
      )}
    </Flex>
  );
}
