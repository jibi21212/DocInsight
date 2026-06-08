import { useEffect, useRef } from "react";
import { EventsOn } from "../../wailsjs/runtime/runtime";

export interface AppEvent {
  type: string;
  data: Record<string, unknown>;
}

/**
 * Subscribe to one or more Wails runtime events. The handler receives a
 * { type, data } shape for parity with the old SSE consumer. Replaces useSSE.
 */
export function useEvents(types: string[], onEvent: (e: AppEvent) => void) {
  const ref = useRef(onEvent);
  ref.current = onEvent;
  const key = types.join("|");

  useEffect(() => {
    const cancels = types.map((t) =>
      EventsOn(t, (data: unknown) =>
        ref.current({ type: t, data: (data ?? {}) as Record<string, unknown> }),
      ),
    );
    return () => cancels.forEach((cancel) => cancel && cancel());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
}
