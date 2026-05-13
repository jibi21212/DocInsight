"use client";

import { useEffect, useRef } from "react";

interface SSEEvent {
  type: string;
  data: Record<string, unknown>;
}

export function useSSE(onEvent: (event: SSEEvent) => void) {
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  useEffect(() => {
    const apiBase = process.env.NEXT_PUBLIC_API_URL || "";
    const url = `${apiBase}/api/events`;

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    function connect() {
      es = new EventSource(url);

      es.onmessage = (e) => {
        try {
          const parsed: SSEEvent = JSON.parse(e.data);
          onEventRef.current(parsed);
        } catch {
          // Ignore malformed events
        }
      };

      es.onerror = () => {
        es?.close();
        // Reconnect after 5 seconds
        reconnectTimer = setTimeout(connect, 5000);
      };
    }

    connect();

    return () => {
      es?.close();
      if (reconnectTimer) clearTimeout(reconnectTimer);
    };
  }, []);
}
