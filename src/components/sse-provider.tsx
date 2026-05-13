"use client";

import { useCallback, useState } from "react";
import { useSSE } from "@/hooks/use-sse";
import { ToastContainer, type Toast } from "./toast-notification";

let toastIdCounter = 0;

export function SSEProvider() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const handleDismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  useSSE(
    useCallback((event) => {
      const id = String(++toastIdCounter);
      const data = event.data as Record<string, string>;

      switch (event.type) {
        case "document.completed":
          setToasts((prev) => [
            ...prev.slice(-4), // Keep max 5 toasts
            {
              id,
              type: "success",
              title: "Document ready",
              message: data.name
                ? `"${data.name}" has been processed successfully.`
                : undefined,
            },
          ]);
          break;

        case "document.failed":
          setToasts((prev) => [
            ...prev.slice(-4),
            {
              id,
              type: "error",
              title: "Processing failed",
              message: data.name
                ? `"${data.name}": ${data.error || "Unknown error"}`
                : data.error || undefined,
            },
          ]);
          break;
      }
    }, [])
  );

  return <ToastContainer toasts={toasts} onDismiss={handleDismiss} />;
}
