import { formatSidebarElapsedTime } from "@/lib/i18n/formats";
import type { AgentProfileOption } from "@/lib/state/slices";
import { isLiveErrand, type Errand } from "@/lib/types/errands";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

const NANOS_PER_MS = BigInt(1_000_000);

function timestampMs(value: string | null | undefined): number | null {
  const nanos = parseStrictRfc3339Timestamp(value ?? undefined);
  return nanos === null ? null : Number(nanos / NANOS_PER_MS);
}

/**
 * Compact elapsed time of an errand: up to now while live, and from creation
 * to settlement once settled. Empty for a malformed creation time.
 */
export function errandElapsedLabel(
  errand: Pick<Errand, "status" | "created_at" | "settled_at" | "updated_at">,
  nowMs: number,
): string {
  const startMs = timestampMs(errand.created_at);
  if (startMs === null) return "";
  const endMs = isLiveErrand(errand)
    ? nowMs
    : (timestampMs(errand.settled_at) ?? timestampMs(errand.updated_at) ?? nowMs);
  return formatSidebarElapsedTime(new Date(startMs).toISOString(), Math.max(startMs, endMs));
}

export type ErrandProfileDisplay = {
  /** Profile name, or the name the start call reported; null when unknown. */
  name: string | null;
  /** Configured start model; null when the agent default applies. */
  model: string | null;
  /** Agent registry slug for the agent logo. */
  agentName: string | null;
};

/** Resolves an errand's profile from the agent profile store, with a reported-name fallback. */
export function resolveErrandProfile(
  profiles: readonly AgentProfileOption[],
  profileId: string | null | undefined,
  fallbackName?: string | null,
): ErrandProfileDisplay {
  const found = profileId ? profiles.find((item) => item.id === profileId) : undefined;
  if (!found) return { name: fallbackName || null, model: null, agentName: null };
  const parts = found.label.split(" \u2022 ");
  return {
    name: parts[1] || parts[0] || fallbackName || null,
    model: found.model || null,
    agentName: found.agent_name || null,
  };
}
