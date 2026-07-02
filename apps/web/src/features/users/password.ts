import type { FormEvent } from "react";
import { apiPost } from "../../lib/api";

export async function submitChangePassword(event: FormEvent<HTMLFormElement>): Promise<void> {
  event.preventDefault();
  const formElement = event.currentTarget;
  const form = new FormData(formElement);
  const currentPassword = String(form.get("current_password") ?? "");
  const newPassword = String(form.get("new_password") ?? "");
  if (newPassword !== String(form.get("confirm_password") ?? "")) {
    throw new Error("两次输入的新密码不一致");
  }
  await apiPost("/auth/password", {
    current_password: currentPassword,
    new_password: newPassword,
  });
  formElement.reset();
}
