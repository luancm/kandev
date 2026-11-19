import type { DockviewApi } from "dockview-react";

type PendingRestore = (ordinarySessionIds: readonly string[]) => void;

const pendingRestores = new WeakMap<DockviewApi, PendingRestore>();

/** Keep the original restore source available until ordinary identities load. */
export function holdDockviewSessionRestore(api: DockviewApi, restore: PendingRestore): void {
  pendingRestores.set(api, restore);
}

/** Re-run a held restore once its task's ordinary session identity is authoritative. */
export function releaseDockviewSessionRestore(
  api: DockviewApi,
  ordinarySessionIds: readonly string[],
): boolean {
  const restore = pendingRestores.get(api);
  if (!restore) return false;
  pendingRestores.delete(api);
  restore(ordinarySessionIds);
  return true;
}

export function hasHeldDockviewSessionRestore(api: DockviewApi): boolean {
  return pendingRestores.has(api);
}
