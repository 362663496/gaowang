import { apiPost } from "../../lib/api";

export type ChangePasswordInput = {
  current_password: string;
  new_password: string;
  confirm_password: string;
};

export async function changePassword(input: ChangePasswordInput): Promise<void> {
  if (input.new_password !== input.confirm_password) {
    throw new Error("两次输入的新密码不一致");
  }
  await apiPost("/auth/password", {
    current_password: input.current_password,
    new_password: input.new_password,
  });
}
