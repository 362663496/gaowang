"use client";

import { Alert, App, Button, Card, Col, Descriptions, Flex, Form, Input, Row, Space, Tag, Typography } from "antd";
import { useCallback, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import type { AppSettings } from "@/features/types";
import { useSession } from "@/components/layout/session-context";
import { copyText, createApiToken, getApiToken, mcpConfigJson, revokeApiToken, type ApiTokenInfo } from "@/features/users/api-token";
import { changePassword, type ChangePasswordInput } from "@/features/users/password";
import { formatDateTime } from "@/lib/format";
import { apiGet, apiPost } from "@/lib/api";

type SettingsValues = { backup_email_recipient: string };

export default function SettingsPage() {
  const { message, modal } = App.useApp();
  const { user, hasPermission } = useSession();
  const canReadSettings = hasPermission("setting.read");
  const canUpdateSettings = hasPermission("setting.update");
  const [passwordForm] = Form.useForm<ChangePasswordInput>();
  const [settingsForm] = Form.useForm<SettingsValues>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [token, setToken] = useState<ApiTokenInfo | null>(null);
  const [secret, setSecret] = useState("");
  const [tokenLoading, setTokenLoading] = useState(true);
  const [tokenSaving, setTokenSaving] = useState(false);

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

  const loadToken = useCallback(async () => {
    setTokenLoading(true);
    setError("");
    try {
      const data = await getApiToken();
      setToken(data.token);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载 API Token 失败");
    } finally {
      setTokenLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadSettings();
  }, [loadSettings]);

  useEffect(() => {
    void loadToken();
  }, [loadToken]);

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
      <PageHeader description="查看当前账号、修改密码、管理 API Token，以及维护备份邮件设置。" title="设置" />
      {error ? (
        <Alert
          action={<Button size="small" onClick={() => { void loadSettings(); void loadToken(); }}>重试</Button>}
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
          <Card loading={tokenLoading} title="API Token / AI">
            <ApiTokenPanel
              origin={typeof window === "undefined" ? "" : window.location.origin}
              saving={tokenSaving}
              secret={secret}
              token={token}
              onCopy={async (text, okMessage) => {
                try {
                  await copyText(text);
                  message.success(okMessage);
                } catch (err) {
                  setError(err instanceof Error ? err.message : "复制失败，请手动选择文本");
                }
              }}
              onCreate={async () => {
                setTokenSaving(true);
                setError("");
                try {
                  const data = await createApiToken();
                  setToken(data.token);
                  setSecret(data.secret);
                  message.success("Token 已创建，请立即复制，刷新后将无法再看到完整值");
                } catch (err) {
                  setError(err instanceof Error ? err.message : "创建 Token 失败");
                } finally {
                  setTokenSaving(false);
                }
              }}
              onRegenerate={() => {
                modal.confirm({
                  title: "重新生成 Token？",
                  content: "旧 Token 会立即失效，已配置的 AI 客户端需要换成新值。",
                  okText: "重新生成",
                  okButtonProps: { danger: true },
                  onOk: async () => {
                    setTokenSaving(true);
                    setError("");
                    try {
                      const data = await createApiToken();
                      setToken(data.token);
                      setSecret(data.secret);
                      message.success("Token 已重新生成，请立即复制");
                    } catch (err) {
                      setError(err instanceof Error ? err.message : "重新生成 Token 失败");
                    } finally {
                      setTokenSaving(false);
                    }
                  },
                });
              }}
              onRevoke={() => {
                modal.confirm({
                  title: "撤销 Token？",
                  content: "撤销后 AI 和接口调用会立即失败，需要重新创建。",
                  okText: "撤销",
                  okButtonProps: { danger: true },
                  onOk: async () => {
                    setTokenSaving(true);
                    setError("");
                    try {
                      await revokeApiToken();
                      setToken(null);
                      setSecret("");
                      message.success("Token 已撤销");
                    } catch (err) {
                      setError(err instanceof Error ? err.message : "撤销 Token 失败");
                    } finally {
                      setTokenSaving(false);
                    }
                  },
                });
              }}
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

function ApiTokenPanel(props: {
  origin: string;
  saving: boolean;
  secret: string;
  token: ApiTokenInfo | null;
  onCopy: (text: string, okMessage: string) => Promise<void>;
  onCreate: () => void;
  onRegenerate: () => void;
  onRevoke: () => void;
}) {
  if (!props.token) {
    return (
      <Flex gap={12} vertical>
        <Typography.Paragraph type="secondary">
          创建一把个人 Token，交给 Claude / Cursor 等 AI。它们通过远程 MCP 用中文帮你查库存、入库和出库。完整值只显示一次。
        </Typography.Paragraph>
        <Button loading={props.saving} type="primary" onClick={props.onCreate}>创建 Token</Button>
      </Flex>
    );
  }

  const mcpText = props.secret ? mcpConfigJson(props.origin, props.secret) : "";

  return (
    <Flex gap={12} vertical>
      <Descriptions
        column={1}
        items={[
          { key: "prefix", label: "Token", children: props.token.prefix + "…" },
          { key: "created", label: "创建时间", children: formatDateTime(props.token.created_at) },
          { key: "used", label: "最近使用", children: formatDateTime(props.token.last_used_at) },
        ]}
      />
      {props.secret ? (
        <Alert
          showIcon
          type="warning"
          message="请立即复制完整 Token。刷新页面后将无法再看到。"
          description={
            <Flex gap={8} vertical>
              <Typography.Paragraph style={{ marginBottom: 0 }}>
                <Typography.Text code>{props.secret}</Typography.Text>
              </Typography.Paragraph>
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                把它贴到 AI 客户端的 MCP 配置里，例如 Cursor 的 mcp.json：
              </Typography.Paragraph>
              <Typography.Paragraph>
                <pre style={{ margin: 0, whiteSpace: "pre-wrap" }}>{mcpText}</pre>
              </Typography.Paragraph>
              <Space wrap>
                <Button onClick={() => void props.onCopy(props.secret, "Token 已复制")}>复制 Token</Button>
                <Button onClick={() => void props.onCopy(mcpText, "MCP 配置已复制")}>复制 MCP 配置</Button>
              </Space>
            </Flex>
          }
        />
      ) : (
        <Typography.Paragraph type="secondary">完整 Token 只在创建或重新生成时显示一次。丢失请重新生成。</Typography.Paragraph>
      )}
      <Space wrap>
        <Button loading={props.saving} onClick={props.onRegenerate}>重新生成</Button>
        <Button danger loading={props.saving} onClick={props.onRevoke}>撤销</Button>
      </Space>
    </Flex>
  );
}
