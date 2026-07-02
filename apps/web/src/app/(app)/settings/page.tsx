"use client";

import { FormEvent, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/fields";
import { MessageBar } from "@/components/ui/message";
import { ErrorBlock } from "@/components/ui/state";
import type { AppSettings } from "@/features/types";
import { submitChangePassword } from "@/features/users/password";
import { useMessage } from "@/features/use-message";
import { apiGet, apiPost, readDevSession, type Role } from "@/lib/api";

export default function SettingsPage() {
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<Role>("admin");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [backupEmailRecipient, setBackupEmailRecipient] = useState("");
  const { message, show } = useMessage();

  async function loadSettings() {
    try {
      const data = await apiGet<{ settings: AppSettings }>("/settings");
      setBackupEmailRecipient(data.settings.backup_email_recipient);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载设置失败");
    }
  }

  useEffect(() => {
    const session = readDevSession();
    setUserId(session.userId);
    setRole(session.role);
    void loadSettings();
  }, []);

  async function changePassword(event: FormEvent<HTMLFormElement>) {
    setSaving(true);
    setError("");
    try {
      await submitChangePassword(event);
      show("密码已更新");
    } catch (err) {
      setError(err instanceof Error ? err.message : "修改密码失败");
    } finally {
      setSaving(false);
    }
  }

  async function saveSettings(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSettingsSaving(true);
    setError("");
    try {
      const data = await apiPost<{ settings: AppSettings }>("/settings", { backup_email_recipient: backupEmailRecipient });
      setBackupEmailRecipient(data.settings.backup_email_recipient);
      show("备份设置已保存");
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存设置失败");
    } finally {
      setSettingsSaving(false);
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="设置" description="查看当前账号并修改自己的密码。" />
      {error ? <ErrorBlock message={error} /> : null}
      <div className="grid gap-4 xl:grid-cols-2">
        <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
          <h2 className="font-semibold">当前账号</h2>
          <div className="mt-4 grid gap-3 text-sm">
            <div>
              <div className="font-medium">用户 ID</div>
              <div className="mt-1 break-all rounded-md border border-[var(--border-subtle)] bg-black/[0.02] px-3 py-2 font-mono text-xs text-[var(--text-secondary)]">{userId}</div>
            </div>
            <div>
              <div className="font-medium">角色</div>
              <div className="mt-1 rounded-md border border-[var(--border-subtle)] bg-black/[0.02] px-3 py-2 text-[var(--text-secondary)]">{role}</div>
            </div>
          </div>
        </section>

        <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
          <h2 className="font-semibold">修改密码</h2>
          <form className="mt-4 grid gap-3" onSubmit={changePassword}>
            <Field label="当前密码"><Input minLength={8} name="current_password" required type="password" /></Field>
            <Field label="新密码"><Input minLength={8} name="new_password" required type="password" /></Field>
            <Field label="确认新密码"><Input minLength={8} name="confirm_password" required type="password" /></Field>
            <div><Button loading={saving} type="submit">更新密码</Button></div>
          </form>
        </section>

        <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
          <h2 className="font-semibold">备份设置</h2>
          <form className="mt-4 grid gap-3" onSubmit={saveSettings}>
            <Field label="备份邮件收件人">
              <Input name="backup_email_recipient" required type="email" value={backupEmailRecipient} onChange={(event) => setBackupEmailRecipient(event.target.value)} />
            </Field>
            <div><Button loading={settingsSaving} type="submit">保存设置</Button></div>
          </form>
        </section>
      </div>
      <MessageBar message={message} />
    </div>
  );
}
