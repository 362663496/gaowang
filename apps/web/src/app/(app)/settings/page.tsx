"use client";

import { Alert, App, Button, Card, Col, Descriptions, Flex, Form, Input, Row, Tag } from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import type { AppSettings } from "@/features/types";
import { useSession } from "@/components/layout/session-context";
import { changePassword, type ChangePasswordInput } from "@/features/users/password";
import { apiGet, apiPost } from "@/lib/api";

type SettingsValues = { backup_email_recipient: string };

export default function SettingsPage() {
  const { message } = App.useApp();
  const { user, hasPermission } = useSession();
  const canReadSettings = hasPermission("setting.read");
  const canUpdateSettings = hasPermission("setting.update");
  const [passwordForm] = Form.useForm<ChangePasswordInput>();
  const [settingsForm] = Form.useForm<SettingsValues>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);

  const loadSettings = useCallback(async () => {
    if (!canReadSettings) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const data = await apiGet<{ settings: AppSettings }>("/settings");
      settingsForm.setFieldsValue({ backup_email_recipient: data.settings.backup_email_recipient });
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载设置失败");
    } finally {
      setLoading(false);
    }
  }, [canReadSettings, settingsForm]);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  async function submitPassword(values: ChangePasswordInput) {
    setSaving(true);
    setError("");
    try {
      await changePassword(values);
      passwordForm.resetFields();
      message.success("密码已更新，请使用新密码重新登录");
      window.location.assign("/login");
    } catch (err) {
      setError(err instanceof Error ? err.message : "修改密码失败");
    } finally {
      setSaving(false);
    }
  }

  async function saveSettings(values: SettingsValues) {
    setSettingsSaving(true);
    setError("");
    try {
      const data = await apiPost<{ settings: AppSettings }>("/settings", values);
      settingsForm.setFieldsValue({ backup_email_recipient: data.settings.backup_email_recipient });
      message.success("备份设置已保存");
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存设置失败");
    } finally {
      setSettingsSaving(false);
    }
  }

  return (
    <Flex gap={20} vertical>
      <PageHeader description="查看当前账号、修改密码和维护备份邮件设置。" title="设置" />
      {error ? (
        <Alert
          action={<Button size="small" onClick={() => void loadSettings()}>重试</Button>}
          closable
          message={error}
          showIcon
          type="error"
          onClose={() => setError("")}
        />
      ) : null}
      <Row gutter={[16, 16]}>
        <Col xl={12} xs={24}>
          <Card title="当前账号">
            <Descriptions
              column={1}
              items={[
                { key: "name", label: "用户名", children: user?.name || "-" },
                { key: "email", label: "邮箱", children: user?.email || "-" },
                { key: "role", label: "角色", children: <Tag color={user?.role === "admin" ? "purple" : "blue"}>{user?.role}</Tag> },
              ]}
            />
          </Card>
        </Col>
        <Col xl={12} xs={24}>
          <Card title="修改密码">
            <Form<ChangePasswordInput> form={passwordForm} layout="vertical" requiredMark={false} onFinish={submitPassword}>
              <Form.Item label="当前密码" name="current_password" rules={[{ required: true, min: 8, message: "请输入至少 8 位当前密码" }]}><Input.Password /></Form.Item>
              <Form.Item label="新密码" name="new_password" rules={[{ required: true, min: 8, message: "请输入至少 8 位新密码" }]}><Input.Password /></Form.Item>
              <Form.Item
                dependencies={["new_password"]}
                label="确认新密码"
                name="confirm_password"
                rules={[
                  { required: true, message: "请再次输入新密码" },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      return !value || getFieldValue("new_password") === value
                        ? Promise.resolve()
                        : Promise.reject(new Error("两次输入的新密码不一致"));
                    },
                  }),
                ]}
              ><Input.Password /></Form.Item>
              <Button htmlType="submit" loading={saving} type="primary">更新密码</Button>
            </Form>
          </Card>
        </Col>
        {canReadSettings ? (
          <Col xl={12} xs={24}>
            <Card loading={loading} title="备份设置">
              <Form<SettingsValues> form={settingsForm} layout="vertical" requiredMark={false} onFinish={saveSettings}>
                <Form.Item label="备份邮件收件人" name="backup_email_recipient" rules={[{ required: true, type: "email", message: "请输入有效邮箱" }]}>
                  <Input disabled={!canUpdateSettings} />
                </Form.Item>
                {canUpdateSettings ? <Button htmlType="submit" loading={settingsSaving} type="primary">保存设置</Button> : null}
              </Form>
            </Card>
          </Col>
        ) : null}
      </Row>
    </Flex>
  );
}
