import type { ReviewSource, SourceCounts } from "@/hooks/domains/session/use-review-sources";

export type TabId = ReviewSource;

/** Tab order rendered in the bar (left → right). */
const TAB_ORDER: TabId[] = ["uncommitted", "committed", "pr"];

export const STORAGE_KEY = "mobile-changes-source";

/**
 * The tabs that should be visible in the bar given the current source
 * counts. Sources with zero files are hidden; the PR tab is shown
 * whenever a PR exists for the task even if its diff hasn't loaded yet
 * (keyed on `hasPR` to avoid reflow when the PR diff hydrates).
 */
export function availableTabs(counts: SourceCounts, hasPR: boolean): TabId[] {
  const out: TabId[] = [];
  for (const id of TAB_ORDER) {
    if (id === "pr") {
      if (hasPR) out.push("pr");
      continue;
    }
    if (counts[id] > 0) out.push(id);
  }
  return out;
}

/**
 * Decide which tab to open with. Falls back through:
 *   1. saved value (if still valid)
 *   2. "uncommitted" (when it has content)
 *   3. first available
 *   4. null (no tabs at all)
 */
export function pickInitialTab(
  saved: string | null,
  counts: SourceCounts,
  hasPR: boolean,
): TabId | null {
  const tabs = availableTabs(counts, hasPR);
  if (tabs.length === 0) return null;
  if (saved && (tabs as string[]).includes(saved)) return saved as TabId;
  if (tabs.includes("uncommitted")) return "uncommitted";
  return tabs[0];
}
