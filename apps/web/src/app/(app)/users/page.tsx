"use client";

import { FormEvent, useEffect, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/fields";
import { MessageBar } from "@/components/ui/message";
import { ErrorBlock } from "@/components/ui/state";
import type { User } from "@/features/types";
import { submitCreateUser } from "@/features/users/create-user";
import { useMessage } from "@/features/use-message";
import { apiGet } from "@/lib/api";

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const { message, show } = useMessage();

  useEffect(() => {
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

  async function createUser(event: FormEvent<HTMLFormElement>) {
    try {
      await submitCreateUser(event);
      show("用户已创建");
      void loadUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建用户失败");
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="用户管理" description="创建后台用户并查看账号信息。" />
      {error ? <ErrorBlock message={error} /> : null}
      <section className="rounded-lg border border-[var(--border-subtle)] bg-white p-4">
        <h2 className="font-semibold">新增用户</h2>
        <form className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4" onSubmit={createUser}>
          <Field label="姓名"><Input name="name" required /></Field>
          <Field label="邮箱"><Input name="email" required type="email" /></Field>
          <Field label="密码"><Input minLength={8} name="password" required type="password" /></Field>
          <Field label="角色">
            <Select name="role" defaultValue="staff">
              <option value="staff">staff</option>
              <option value="admin">admin</option>
            </Select>
          </Field>
          <div className="md:col-span-2 xl:col-span-4"><Button type="submit">创建用户</Button></div>
        </form>
      </section>

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
