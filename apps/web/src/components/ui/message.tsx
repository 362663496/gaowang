import { AlertCircle, CheckCircle2 } from "lucide-react";

export function MessageBar({ message, tone = "success" }: { message: string; tone?: "success" | "error" }) {
  if (!message) {
    return null;
  }
  const isError = tone === "error";
  const Icon = isError ? AlertCircle : CheckCircle2;
  return (
    <div
      className={`fixed bottom-4 right-4 z-50 flex max-w-md items-center gap-2 rounded-lg border bg-white px-4 py-3 text-sm shadow-xl ${isError ? "border-red-200 text-red-800" : "border-emerald-200 text-emerald-800"}`}
      role={isError ? "alert" : "status"}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {message}
    </div>
  );
}
