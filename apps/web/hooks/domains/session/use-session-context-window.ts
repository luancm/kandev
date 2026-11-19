import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { ContextWindowEntry } from "@/lib/state/store";
import {
  parseContextWindowEntry,
  readContextWindowMetadata,
  reconcileContextWindowMetadata,
  sameContextWindowEntry,
  patchContextWindowCompaction,
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
  const compactionReporting = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.compactionReporting : undefined,
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
      compactionReporting,
      source,
      timestamp,
    };
  }, [size, used, remaining, efficiency, compactionCount, compactionReporting, source, timestamp]);

  const sessionMetadata = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.metadata : undefined,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  const metadataContextWindow = useMemo(() => {
    return parseMetadataContextWindow(sessionMetadata);
  }, [sessionMetadata]);

  const setContextWindow = useAppStore((state) => state.setContextWindow);
  useEffect(() => {
    if (!detailActive || !sessionId || (!contextWindow && contextWindowEpoch > 0)) return;
    if (!sessionMetadata || typeof sessionMetadata !== "object") return;
    const record = sessionMetadata as Record<string, unknown>;
    const next = reconcileContextWindowMetadata({
      current: contextWindow,
      gauge: record.context_window,
      hasGauge: Object.prototype.hasOwnProperty.call(record, "context_window"),
      count: record.context_compaction_count,
      reporting: record.context_compaction_reporting,
      timestamp: undefined,
      allowExplicitClear: false,
    });
    if (!next || sameContextWindowEntry(next, contextWindow)) return;
    setContextWindow(sessionId, next);
  }, [
    detailActive,
    sessionId,
    sessionMetadata,
    contextWindow,
    contextWindowEpoch,
    setContextWindow,
  ]);

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

  if (contextWindow) {
    const record = sessionMetadata as Record<string, unknown> | undefined;
    return (
      patchContextWindowCompaction(
        contextWindow,
        record?.context_compaction_count,
        record?.context_compaction_reporting,
      ) ?? contextWindow
    );
  }
  return contextWindowEpoch === 0 ? metadataContextWindow : undefined;
}

function parseMetadataContextWindow(sessionMetadata: unknown): ContextWindowEntry | undefined {
  const metadata = readContextWindowMetadata(sessionMetadata);
  if (!metadata.present) return undefined;
  return (
    parseContextWindowEntry(
      metadata.contextWindow,
      undefined,
      metadata.compactionCount,
      (sessionMetadata as Record<string, unknown>).context_compaction_reporting,
    ) ?? undefined
  );
}
