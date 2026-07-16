import type { User } from "../types";
import { apiPost } from "../../lib/api";

export type CreateUserInput = {
  name: string;
  email: string;
  password: string;
  role: "admin" | "staff";
};

export async function createUser(input: CreateUserInput): Promise<User> {
  const data = await apiPost<{ item: User }>("/users", {
    name: input.name,
    email: input.email,
    password: input.password,
    role: input.role,
  });
  return data.item;
}
