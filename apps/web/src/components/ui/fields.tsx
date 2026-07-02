import * as React from "react";
import { cn } from "@/lib/utils";

const control =
  "h-9 w-full rounded-md border border-[var(--border-subtle)] bg-white px-3 text-sm text-[var(--text-primary)] outline-none transition focus:border-[var(--accent-primary)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--accent-primary)_18%,transparent)] disabled:bg-black/[0.03] disabled:text-[var(--text-muted)]";

export function Field({ label, children, error }: { label: string; children: React.ReactNode; error?: string }) {
  return (
    <label className="grid gap-1.5 text-sm">
      <span className="font-medium text-[var(--text-primary)]">{label}</span>
      {children}
      {error ? <span className="text-xs text-[var(--status-error)]">{error}</span> : null}
    </label>
  );
}

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(control, className)} {...props} />;
}

export function Textarea({ className, ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(control, "min-h-20 py-2", className)} {...props} />;
}

export function Select({ className, ...props }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(control, className)} {...props} />;
}
