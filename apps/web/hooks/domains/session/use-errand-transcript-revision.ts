import { useEffect, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

const TRANSCRIPT_REFRESH_INTERVAL_MS = 250;

/** Refreshes the visible transcript once for a burst of committed changes. */
export function useErrandTranscriptRevision(taskId: string, errandId: string, connected: boolean) {
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const client = connected ? getWebSocketClient() : null;
    if (!client) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const off = client.on("errand.transcript_changed", ({ payload }) => {
      if (payload?.task_id !== taskId || payload.errand_id !== errandId || timer !== undefined) {
        return;
      }
      timer = setTimeout(() => {
        timer = undefined;
        setRevision((current) => current + 1);
      }, TRANSCRIPT_REFRESH_INTERVAL_MS);
    });
    const unsubscribe = client.subscribe(taskId);
    return () => {
      clearTimeout(timer);
      off();
      unsubscribe();
    };
  }, [connected, errandId, taskId]);
  return revision;
}
