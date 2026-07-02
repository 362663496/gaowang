import type { FormEvent } from "react";
import type { User } from "../types";
import { apiPost } from "../../lib/api";

export async function submitCreateUser(event: FormEvent<HTMLFormElement>): Promise<User> {
  event.preventDefault();
  const formElement = event.currentTarget;
  const form = new FormData(formElement);
  const data = await apiPost<{ item: User }>("/users", {
    name: String(form.get("name") ?? ""),
    email: String(form.get("email") ?? ""),
    password: String(form.get("password") ?? ""),
    role: String(form.get("role") ?? "staff"),
  });
  formElement.reset();
  return data.item;
}
