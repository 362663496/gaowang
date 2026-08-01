"use client";

import { DeleteOutlined } from "@ant-design/icons";
import { Alert, App, Button, Card, Col, Flex, Form, Input, Row, Select, Table, Tag, type TableProps } from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { useSession } from "@/components/layout/session-context";
import { initialPagination, tablePagination } from "@/features/pagination";
import type { Paginated, User } from "@/features/types";
import { createUser, type CreateUserInput } from "@/features/users/create-user";
import { apiGet, request } from "@/lib/api";

export default function UsersPage() {
  const { message, modal } = App.useApp();
  const { user, hasPermission } = useSession();
  const canDelete = hasPermission("user.delete");
  const [form] = Form.useForm<CreateUserInput>();
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deletingID, setDeletingID] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(initialPagination);

  const loadUsers = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await apiGet<Paginated<User>>(`/users?page=${page}`);
      setUsers(data.items);
      setPagination(data.pagination);
      if (data.pagination.page !== page) setPage(data.pagination.page);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载用户失败");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  async function submit(values: CreateUserInput) {
    setSaving(true);
    setError("");
    try {
      await createUser(values);
      form.resetFields();
      message.success("用户已创建");
      if (page === 1) void loadUsers();
      else setPage(1);
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建用户失败");
    } finally {
      setSaving(false);
    }
  }

  function confirmDelete(target: User) {
    modal.confirm({
      title: `确认删除用户“${target.name}”？`,
      content: "删除后将停用账号并撤销全部会话与权限，历史流水和审计记录会保留，用户名和邮箱不能再次使用。",
      okText: "确认删除",
      cancelText: "取消",
      okButtonProps: { danger: true },
      async onOk() {
        setDeletingID(target.id);
        setError("");
        try {
          await request<void>(`/users/${target.id}`, { method: "DELETE" });
          message.success("用户已删除");
          await loadUsers();
        } catch (err) {
          setError(err instanceof Error ? err.message : "删除用户失败");
        } finally {
          setDeletingID(null);
        }
      },
    });
  }

  const columns: TableProps<User>["columns"] = [
    { title: "姓名", dataIndex: "name", width: 160, render: (value: string) => <strong>{value}</strong> },
    { title: "邮箱", dataIndex: "email", width: 260 },
    { title: "角色", dataIndex: "role", width: 110, render: (value: User["role"]) => <Tag color={value === "admin" ? "purple" : "blue"}>{value}</Tag> },
    { title: "ID", dataIndex: "id", render: (value: string) => <span className="mono muted">{value}</span> },
    {
      title: "操作",
      key: "actions",
      fixed: "right",
      width: 100,
      render: (_, target) => canDelete && target.id !== user?.id ? (
        <Button
          danger
          disabled={deletingID !== null}
          icon={<DeleteOutlined />}
          loading={deletingID === target.id}
          size="small"
          type="text"
          onClick={() => confirmDelete(target)}
        >删除</Button>
      ) : null,
    },
  ];

  return (
    <Flex gap={20} vertical>
      <PageHeader description="创建后台用户并查看账号信息。" title="用户管理" />
      {error ? <Alert closable message={error} showIcon type="error" onClose={() => setError("")} /> : null}
      <Card title="新增用户">
        <Form<CreateUserInput> form={form} initialValues={{ role: "staff" }} layout="vertical" requiredMark={false} onFinish={submit}>
          <Row gutter={14}>
            <Col lg={6} sm={12} xs={24}><Form.Item label="用户名" name="name" rules={[{ required: true, message: "请输入用户名" }]}><Input /></Form.Item></Col>
            <Col lg={6} sm={12} xs={24}><Form.Item label="邮箱" name="email" rules={[{ required: true, type: "email", message: "请输入有效邮箱" }]}><Input /></Form.Item></Col>
            <Col lg={6} sm={12} xs={24}><Form.Item label="密码" name="password" rules={[{ required: true, min: 8, message: "请输入至少 8 位密码" }]}><Input.Password /></Form.Item></Col>
            <Col lg={6} sm={12} xs={24}>
              <Form.Item label="角色" name="role" rules={[{ required: true }]}>
                <Select options={[{ value: "staff", label: "staff" }, { value: "admin", label: "admin" }]} />
              </Form.Item>
            </Col>
          </Row>
          <Button htmlType="submit" loading={saving} type="primary">创建用户</Button>
        </Form>
      </Card>
      <Card className="table-card" title="用户列表">
        <Table<User>
          columns={columns}
          dataSource={users}
          loading={loading}
          pagination={tablePagination(pagination, setPage)}
          rowKey="id"
          scroll={{ x: 860 }}
        />
      </Card>
    </Flex>
  );
}
