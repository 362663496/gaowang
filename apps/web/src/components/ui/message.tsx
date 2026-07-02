import { CheckCircle2 } from "lucide-react";

export function MessageBar({ message }: { message: string }) {
  if (!message) {
    return null;
  }
  return (
    <div className="fixed bottom-4 right-4 z-50 flex items-center gap-2 rounded-lg border border-emerald-200 bg-white px-4 py-3 text-sm text-emerald-800 shadow-xl">
      <CheckCircle2 className="h-4 w-4" />
      {message}
    </div>
  );
}
