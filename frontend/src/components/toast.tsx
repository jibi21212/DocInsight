import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, XCircle, X } from "lucide-react";
import { useEvents } from "@/hooks/use-events";

export interface Toast {
  id: string;
  type: "success" | "error";
  title: string;
  message?: string;
}

function ToastItem({ toast, onDismiss }: { toast: Toast; onDismiss: (id: string) => void }) {
  useEffect(() => {
    const timer = setTimeout(() => onDismiss(toast.id), 5000);
    return () => clearTimeout(timer);
  }, [toast.id, onDismiss]);

  const isSuccess = toast.type === "success";
  return (
    <div
      className={`flex items-start gap-3 rounded-lg border p-4 shadow-lg transition-all ${
        isSuccess
          ? "border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-900/30"
          : "border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/30"
      }`}
    >
      {isSuccess ? (
        <CheckCircle2 size={18} className="mt-0.5 shrink-0 text-green-600 dark:text-green-400" />
      ) : (
        <XCircle size={18} className="mt-0.5 shrink-0 text-red-600 dark:text-red-400" />
      )}
      <div className="min-w-0 flex-1">
        <p className={`text-sm font-medium ${isSuccess ? "text-green-800 dark:text-green-300" : "text-red-800 dark:text-red-300"}`}>
          {toast.title}
        </p>
        {toast.message && (
          <p className={`mt-0.5 text-xs ${isSuccess ? "text-green-600 dark:text-green-400" : "text-red-600 dark:text-red-400"}`}>
            {toast.message}
          </p>
        )}
      </div>
      <button
        onClick={() => onDismiss(toast.id)}
        className="shrink-0 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300"
      >
        <X size={14} />
      </button>
    </div>
  );
}

let toastIdCounter = 0;

/** Bridges backend Wails events to toast notifications (replaces the SSE provider). */
export function Toaster() {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const dismiss = useCallback((id: string) => setToasts((prev) => prev.filter((t) => t.id !== id)), []);

  useEvents(
    ["document.completed", "document.failed", "sidecar.error"],
    useCallback((event) => {
      const id = String(++toastIdCounter);
      const data = event.data as Record<string, string>;
      if (event.type === "document.completed") {
        setToasts((prev) => [
          ...prev.slice(-4),
          { id, type: "success", title: "Document ready", message: data.name ? `"${data.name}" has been processed.` : undefined },
        ]);
      } else if (event.type === "document.failed") {
        setToasts((prev) => [
          ...prev.slice(-4),
          { id, type: "error", title: "Processing failed", message: data.name ? `"${data.name}": ${data.error || "Unknown error"}` : data.error || undefined },
        ]);
      } else if (event.type === "sidecar.error") {
        setToasts((prev) => [
          ...prev.slice(-4),
          { id, type: "error", title: "Embeddings unavailable", message: "The embedding service didn't start. Run setup.ps1, then restart." },
        ]);
      }
    }, []),
  );

  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onDismiss={dismiss} />
      ))}
    </div>
  );
}
