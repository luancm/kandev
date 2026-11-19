import { useCallback, useMemo, useState } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import * as errandApi from "@/lib/api/domains/errand-api";

export type ErrandAction = "cancel" | "cancel_all" | "send_result";

export type ErrandActions = {
  /** Sends `errand.cancel`; the errand ID is its session ID. */
  cancelErrand: (sessionId: string) => Promise<void>;
  /** Sends `errand.cancel_all`; resolves with how many errands were cancelled. */
  cancelAllErrands: (taskId: string) => Promise<number>;
  /** Sends `errand.send_result` for the errand's latest settled turn. */
  sendErrandResult: (sessionId: string) => Promise<void>;
  /** Whether a request this hook sent for the action and target is still in flight. */
  isPending: (action: ErrandAction, targetId: string) => boolean;
};

type AppStore = ReturnType<typeof useAppStoreApi>;
const inflightByStore = new WeakMap<AppStore, Map<string, Promise<unknown>>>();

function storeInflight(store: AppStore) {
  let inflight = inflightByStore.get(store);
  if (!inflight) {
    inflight = new Map();
    inflightByStore.set(store, inflight);
  }
  return inflight;
}

function actionKey(action: ErrandAction, targetId: string): string {
  return `${action}:${targetId}`;
}

/** Concurrent callers of the same action and target share one request. */
function shareInflight<T>(store: AppStore, key: string, send: () => Promise<T>): Promise<T> {
  const inflight = storeInflight(store);
  const current = inflight.get(key);
  if (current) return current as Promise<T>;
  const request = send().finally(() => inflight.delete(key));
  inflight.set(key, request);
  return request;
}

/**
 * User errand actions. Errand state changes arrive through `errand.updated`,
 * so callers only handle the request outcome.
 */
export function useErrandActions(): ErrandActions {
  const store = useAppStoreApi();
  const [pendingKeys, setPendingKeys] = useState<ReadonlySet<string>>(() => new Set());

  const run = useCallback(
    <T>(action: ErrandAction, targetId: string, send: () => Promise<T>): Promise<T> => {
      const key = actionKey(action, targetId);
      setPendingKeys((keys) => new Set(keys).add(key));
      return shareInflight(store, key, send).finally(() =>
        setPendingKeys((keys) => {
          const next = new Set(keys);
          next.delete(key);
          return next;
        }),
      );
    },
    [store],
  );

  const isPending = useCallback(
    (action: ErrandAction, targetId: string) => pendingKeys.has(actionKey(action, targetId)),
    [pendingKeys],
  );

  return useMemo(
    () => ({
      cancelErrand: (sessionId: string) =>
        run("cancel", sessionId, () => errandApi.cancelErrand(sessionId)),
      cancelAllErrands: (taskId: string) =>
        run("cancel_all", taskId, () => errandApi.cancelAllErrands(taskId)),
      sendErrandResult: (sessionId: string) =>
        run("send_result", sessionId, () => errandApi.sendErrandResult(sessionId)),
      isPending,
    }),
    [run, isPending],
  );
}
