import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { ContextWindowEntry } from "@/lib/state/store";
import {
  parseContextWindowEntry,
  readContextWindowMetadata,
} from "@/lib/state/slices/session-runtime/context-window";

export function useSessionContextWindow(
  sessionId: string | null,
  detailActive = true,
): ContextWindowEntry | undefined {
  // Subscribe to individual primitive values to ensure reactivity
  const size = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.size : undefined,
  );
  const used = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.used : undefined,
  );
  const remaining = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.remaining : undefined,
  );
  const efficiency = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.efficiency : undefined,
  );
  const compactionCount = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.compactionCount : undefined,
  );
  const source = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.source : undefined,
  );
  const timestamp = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.timestamp : undefined,
  );
  const contextWindowEpoch = useAppStore((state) =>
    sessionId ? (state.taskSessions.contextWindowEpochBySession?.[sessionId] ?? 0) : 0,
  );

  // Memoize the combined object
  const contextWindow = useMemo(() => {
    if (size === undefined) return undefined;
    return {
      size,
      used: used ?? 0,
      remaining: remaining ?? 0,
      efficiency: efficiency ?? 0,
      compactionCount: compactionCount ?? 0,
      source,
      timestamp,
    };
  }, [size, used, remaining, efficiency, compactionCount, source, timestamp]);

  const sessionMetadata = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.metadata : undefined,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  const metadataContextWindow = useMemo(() => {
    const metadata = readContextWindowMetadata(sessionMetadata);
    if (!metadata.present) return undefined;
    return (
      parseContextWindowEntry(metadata.contextWindow, undefined, metadata.compactionCount) ??
      undefined
    );
  }, [sessionMetadata]);

  // Subscribe to session updates via WebSocket
  useEffect(() => {
    if (!detailActive) return;
    if (!sessionId) return;
    if (connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear context window on cleanup - keep it cached
      };
    }
  }, [detailActive, sessionId, connectionStatus]);

  return contextWindow ?? (contextWindowEpoch === 0 ? metadataContextWindow : undefined);
}
