import type { AppState } from "@/lib/state/store";
import type { TaskSessionsResponse } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import { SharedResourceReads, storeReadScopeIdentity } from "./shared-resource-reads";

export type TaskSessionReads = SharedResourceReads<TaskSessionsResponse>;

type TaskSessionReadOwner = { identity: string; reads: TaskSessionReads };
const owners = new WeakMap<StoreApi<AppState>, TaskSessionReadOwner>();

function taskSessionReadScopeIdentity(store: StoreApi<AppState>): string {
  return JSON.stringify([
    storeReadScopeIdentity(store),
    store.getState().connection?.generation ?? 0,
  ]);
}

export function createTaskSessionReads(scopeIsCurrent?: () => boolean): TaskSessionReads {
  return new SharedResourceReads<TaskSessionsResponse>(scopeIsCurrent);
}

export function getTaskSessionReads(store: StoreApi<AppState>): TaskSessionReads {
  const identity = taskSessionReadScopeIdentity(store);
  let owner = owners.get(store);
  if (!owner || owner.identity !== identity) {
    owner?.reads.dispose();
    const nextOwner: TaskSessionReadOwner = {
      identity,
      reads: new SharedResourceReads<TaskSessionsResponse>(
        () => owners.get(store) === nextOwner && taskSessionReadScopeIdentity(store) === identity,
      ),
    };
    owner = nextOwner;
    owners.set(store, owner);
  }
  return owner.reads;
}
