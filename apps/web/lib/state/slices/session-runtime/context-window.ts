import type { ContextWindowEntry } from "./types";

export type ContextWindowMetadata = {
  present: false;
};

export type PresentContextWindowMetadata = {
  present: true;
  contextWindow: unknown;
  compactionCount: unknown;
};

export type ContextWindowMetadataRead = ContextWindowMetadata | PresentContextWindowMetadata;

export function readContextWindowMetadata(metadata: unknown): ContextWindowMetadataRead {
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) {
    return { present: false };
  }

  const record = metadata as Record<string, unknown>;
  if (!Object.prototype.hasOwnProperty.call(record, "context_window")) {
    return { present: false };
  }

  return {
    present: true,
    contextWindow: record.context_window,
    compactionCount: record.context_compaction_count,
  };
}

export function readContextWindowMetadataFromAliases(
  metadata: unknown,
  sessionMetadata: unknown,
): ContextWindowMetadataRead {
  const primary = readContextWindowMetadata(metadata);
  return primary.present ? primary : readContextWindowMetadata(sessionMetadata);
}

export function normalizeContextCompactionCount(value: unknown): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0;
  return Math.max(0, Math.trunc(value));
}

export function parseContextWindowEntry(
  value: unknown,
  timestamp?: string,
  compactionCount?: unknown,
): ContextWindowEntry | null {
  if (!value || typeof value !== "object") return null;

  const contextWindow = value as Record<string, unknown>;
  const source =
    contextWindow.source === "acp" || contextWindow.source === "api"
      ? contextWindow.source
      : undefined;

  return {
    size: (contextWindow.size as number) ?? 0,
    used: (contextWindow.used as number) ?? 0,
    remaining: (contextWindow.remaining as number) ?? 0,
    efficiency: (contextWindow.efficiency as number) ?? 0,
    compactionCount: normalizeContextCompactionCount(compactionCount),
    source,
    timestamp: timestamp ?? (contextWindow.timestamp as string | undefined),
  };
}

export function updateContextWindowMetadata(
  metadata: unknown,
  entry: ContextWindowEntry | null,
): Record<string, unknown> {
  const current =
    metadata && typeof metadata === "object" && !Array.isArray(metadata)
      ? (metadata as Record<string, unknown>)
      : {};
  if (!entry) return { ...current, context_window: null };

  const contextWindow = {
    size: entry.size,
    used: entry.used,
    remaining: entry.remaining,
    efficiency: entry.efficiency,
    ...(entry.source ? { source: entry.source } : {}),
    ...(entry.timestamp !== undefined ? { timestamp: entry.timestamp } : {}),
  };
  return {
    ...current,
    context_window: contextWindow,
    context_compaction_count: normalizeContextCompactionCount(entry.compactionCount),
  };
}
