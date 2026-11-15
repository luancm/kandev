import type { StoreApi } from "zustand";
import type { AppState } from "../../app-state-types";
import { stateReadScopeIdentity } from "../../shared-resource-reads";
import { invalidateErrandBrowserReads } from "./errand-browser-read-coordinator";

/** Scope changes revoke retained browser data and all outstanding read leases. */
export function observeErrandReadScope(store: StoreApi<AppState>): void {
  let previous = store.getState();
  let identity = stateReadScopeIdentity(previous);
  store.subscribe((state) => {
    const changed =
      state.auth !== previous.auth ||
      state.workspaceContextGeneration !== previous.workspaceContextGeneration;
    previous = state;
    if (!changed) return;
    const next = stateReadScopeIdentity(state);
    if (next === identity) return;
    identity = next;
    invalidateErrandBrowserReads(store);
    store.getState().resetErrandReadScope();
  });
}
