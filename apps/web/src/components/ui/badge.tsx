import type * as React from "react";
import { cn } from "@/lib/utils";

const tones = {
  neutral: "border-black/10 bg-black/[0.04] text-[var(--text-secondary)]",
  success: "border-emerald-600/20 bg-emerald-50 text-emerald-700",
  warning: "border-amber-600/20 bg-amber-50 text-amber-700",
  error: "border-red-600/20 bg-red-50 text-red-700",
  accent: "border-indigo-600/20 bg-indigo-50 text-indigo-700",
};

export function Badge({ children, tone = "neutral" }: { children: React.ReactNode; tone?: keyof typeof tones }) {
  return (
    <span className={cn("inline-flex h-6 items-center rounded-full border px-2 text-xs font-medium", tones[tone])}>
      {children}
    </span>
  );
}
