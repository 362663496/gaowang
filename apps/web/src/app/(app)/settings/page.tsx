"use client";

import { FormEvent, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/fields";
import { MessageBar } from "@/components/ui/message";
import { ErrorBlock } from "@/components/ui/state";
import type { User } from "@/features/types";
import { useMessage } from "@/features/use-message";
import { apiGet, apiPost, readDevSession, type Role, writeDevSession } from "@/lib/api";

export default function SettingsPage() {
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<Role>("admin");
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const { message, show } = useMessage();

  useEffect(() => {
    const session = readDevSession();
    setUserId(session.userId);
    setRole(session.role);
    void loadUsers();
  }, []);

  async function loadUsers() {
    setError("");
    try {
      setUsers((await apiGet<{ items: User[] }>("/users")).items);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载用户失败");
    }
  }

  function saveSession(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    writeDevSession({ userId, role });
    show("身份已保存");
    void loadUsers();
  }

  async function createUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      await apiPost<{ item: User }>("/users", {
        name: String(form.get("name") ?? ""),
        email: String(form.get("email") ?? ""),
        password: String(form.get("password") ?? ""),
        role: String(form.get("role") ?? "staff"),
      });
      event.currentTarget.reset();
      show("用户已创建");
      void loadUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建用户失败");
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="设置" description="开发身份、用户和基础操作入口。" />
      {error ? <ErrorBlock message={error} /> : null}
      <div className="grid gap-4 xl:grid-cols-2">
        <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
          <h2 className="font-semibold">当前操作身份</h2>
          <form className="mt-4 grid gap-3" onSubmit={saveSession}>
            <Field label="用户 ID"><Input value={userId} onChange={(event) => setUserId(event.target.value)} placeholder="任意 UUID，用于开发认证 header" required /></Field>
            <Field label="角色">
              <Select value={role} onChange={(event) => setRole(event.target.value as Role)}>
                <option value="admin">admin</option>
                <option value="staff">staff</option>
              </Select>
            </Field>
            <div><Button type="submit">保存身份</Button></div>
          </form>
        </section>

        <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
          <h2 className="font-semibold">新增用户</h2>
          <form className="mt-4 grid gap-3" onSubmit={createUser}>
            <Field label="姓名"><Input name="name" required /></Field>
            <Field label="邮箱"><Input name="email" required type="email" /></Field>
            <Field label="密码"><Input minLength={8} name="password" required type="password" /></Field>
            <Field label="角色">
              <Select name="role" defaultValue="staff">
                <option value="staff">staff</option>
                <option value="admin">admin</option>
              </Select>
            </Field>
            <div><Button type="submit">创建用户</Button></div>
          </form>
        </section>
      </div>

      <section className="overflow-x-auto rounded-lg border border-[var(--border-subtle)] bg-white">
        <div className="border-b border-[var(--border-subtle)] px-4 py-3 font-medium">用户列表</div>
        <table className="w-full min-w-[640px] text-left text-sm">
          <thead className="border-b border-[var(--border-subtle)] text-xs text-[var(--text-secondary)]">
            <tr><th className="px-4 py-3 font-medium">姓名</th><th className="px-4 py-3 font-medium">邮箱</th><th className="px-4 py-3 font-medium">角色</th><th className="px-4 py-3 font-medium">ID</th></tr>
          </thead>
          <tbody>
            {users.map((user) => (
              <tr className="border-b border-[var(--border-subtle)] last:border-0" key={user.id}>
                <td className="px-4 py-3 font-medium">{user.name}</td>
                <td className="px-4 py-3">{user.email}</td>
                <td className="px-4 py-3">{user.role}</td>
                <td className="px-4 py-3 font-mono text-xs text-[var(--text-secondary)]">{user.id}</td>
              </tr>
            ))}
            {users.length === 0 ? <tr><td className="px-4 py-8 text-center text-[var(--text-secondary)]" colSpan={4}>暂无用户或当前身份无权限</td></tr> : null}
          </tbody>
        </table>
      </section>
      <MessageBar message={message} />
    </div>
  );
}
