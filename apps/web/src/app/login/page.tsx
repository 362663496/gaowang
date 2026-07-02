"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/fields";
import { ErrorBlock } from "@/components/ui/state";
import type { User } from "@/features/types";
import { apiPost, writeDevSession } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      const data = await apiPost<{ user: User }>("/auth/login", {
        login: String(form.get("login") ?? ""),
        password: String(form.get("password") ?? ""),
      });
      writeDevSession({ userId: data.user.id, role: data.user.role });
      router.push("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-[var(--surface-page)] p-4">
      <section className="w-full max-w-sm rounded-lg border border-[var(--border-subtle)] bg-white p-5 shadow-sm">
        <div>
          <h1 className="text-xl font-semibold">登录 Gaowang</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">使用后台用户名或邮箱进入库存系统。</p>
        </div>
        <form className="mt-5 grid gap-4" onSubmit={submit}>
          {error ? <ErrorBlock message={error} /> : null}
          <Field label="用户名或邮箱"><Input name="login" required /></Field>
          <Field label="密码"><Input minLength={8} name="password" required type="password" /></Field>
          <Button className="w-full" loading={saving} type="submit">登录</Button>
        </form>
      </section>
    </main>
  );
}
