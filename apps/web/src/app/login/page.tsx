"use client";

import { LockOutlined, UserOutlined } from "@ant-design/icons";
import { Alert, Button, Card, Flex, Form, Input, Typography } from "antd";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { User } from "@/features/types";
import { apiPost, writeDevSession } from "@/lib/api";

type LoginValues = { login: string; password: string };

export default function LoginPage() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  async function submit(values: LoginValues) {
    setSaving(true);
    setError("");
    try {
      const data = await apiPost<{ user: User }>("/auth/login", values);
      writeDevSession({ userId: data.user.id, role: data.user.role });
      router.push("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Flex align="center" className="login-page" justify="center">
      <Card className="login-card">
        <Flex gap={10} vertical>
          <Typography.Title level={3} style={{ margin: 0 }}>登录 Gaowang</Typography.Title>
          <Typography.Text type="secondary">使用后台用户名或邮箱进入库存系统。</Typography.Text>
        </Flex>
        <Form<LoginValues> layout="vertical" requiredMark={false} style={{ marginTop: 22 }} onFinish={submit}>
          {error ? <Alert message={error} showIcon style={{ marginBottom: 16 }} type="error" /> : null}
          <Form.Item label="用户名或邮箱" name="login" rules={[{ required: true, message: "请输入用户名或邮箱" }]}>
            <Input autoComplete="username" prefix={<UserOutlined />} />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, min: 8, message: "请输入至少 8 位密码" }]}>
            <Input.Password autoComplete="current-password" prefix={<LockOutlined />} />
          </Form.Item>
          <Button block htmlType="submit" loading={saving} type="primary">登录</Button>
        </Form>
      </Card>
    </Flex>
  );
}
