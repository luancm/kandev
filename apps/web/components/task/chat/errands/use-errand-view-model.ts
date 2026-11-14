import { useEffect, useId, useLayoutEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { inspectErrand } from "@/lib/api/domains/errand-api";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { errandBrowserReadCoordinator } from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import {
  selectErrandBySessionId,
  selectErrandBrowserRowBySessionId,
  selectErrandInspection,
} from "@/lib/state/slices/errands/selectors";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { ErrandBrowserRow } from "@/lib/types/errands";
import { selectSessionTabTitle } from "../../session-tab-title";

export type ErrandViewModel = {
  sessionId: string;
  taskId: string;
  /** Bounded current row; absent until summary, history, or targeted inspect supplies it. */
  errand: ErrandBrowserRow | null;
  parentSessionId: string | null;
  parentTitle: string;
  title: string;
  objective: string | null;
  workspacePath: string | null;
  branch: string | null;
};

function useErrandInspectionOwner(args: {
  enabled: boolean;
  taskId: string | null;
  sessionId: string | null | undefined;
  isErrand: boolean;
  ownerId: string;
  store: ReturnType<typeof useAppStoreApi>;
  connectionStatus: string;
  connectionGeneration: number;
  readScopeIdentity: string;
  taskResetEpoch: number;
  observationRevision: number;
}) {
  const {
    enabled,
    taskId,
    sessionId,
    isErrand,
    ownerId,
    store,
    connectionStatus,
    connectionGeneration,
    readScopeIdentity,
    taskResetEpoch,
    observationRevision,
  } = args;
  useEffect(() => {
    if (!enabled || !taskId || !sessionId || !isErrand) return;
    let released = false;
    const existing = selectErrandInspection(store.getState(), sessionId);
    const retainedRevision =
      store.getState().errands.detailObservationRevisionBySessionId?.[sessionId];
    if (
      existing?.scope.task_id === taskId &&
      existing.scope.child_session_id === sessionId &&
      (retainedRevision ?? observationRevision) === observationRevision
    ) {
      store.getState().retainErrandInspection(ownerId, existing);
      return () => store.getState().releaseErrandInspection(ownerId);
    }
    if (connectionStatus !== "connected") return;

    const lease = errandBrowserReadCoordinator(store).acquire(
      {
        kind: "inspection",
        readScopeIdentity,
        taskId,
        childSessionId: sessionId,
        connectionGeneration,
        taskResetEpoch,
      },
      observationRevision,
      (signal) => inspectErrand(taskId, sessionId, signal),
    );
    void lease.promise
      .then((result) => {
        if (
          released ||
          stateReadScopeIdentity(store.getState()) !== readScopeIdentity ||
          (store.getState().errands.browserRevisionByTaskId[taskId] ?? 0) !== observationRevision ||
          result.scope.task_id !== taskId ||
          result.scope.child_session_id !== sessionId ||
          result.errand.task_id !== taskId ||
          result.errand.errand_id !== sessionId ||
          !result.child_session_exists
        ) {
          return;
        }
        store.getState().retainErrandInspection(ownerId, result);
      })
      .catch(() => undefined)
      .finally(lease.release);
    return () => {
      released = true;
      lease.release();
      store.getState().releaseErrandInspection(ownerId);
    };
  }, [
    observationRevision,
    connectionGeneration,
    connectionStatus,
    enabled,
    isErrand,
    ownerId,
    readScopeIdentity,
    sessionId,
    store,
    taskId,
    taskResetEpoch,
  ]);
}

/** View model of the selected child conversation and its targeted inspection. */
export function useErrandViewModel(
  sessionId: string | null | undefined,
  taskIdHint: string | null | undefined,
): ErrandViewModel | null {
  const { t } = useTranslation();
  const enabled = useFeature("errands");
  const store = useAppStoreApi();
  const ownerId = `errand-view-${useId()}`;
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const taskId = session?.task_id ?? taskIdHint ?? null;
  const errand = useAppStore((state) =>
    enabled && taskId && sessionId
      ? selectErrandBrowserRowBySessionId(state, taskId, sessionId)
      : undefined,
  );
  const objective = useAppStore((state) =>
    enabled && taskId && sessionId
      ? (selectErrandBySessionId(state, taskId, sessionId)?.objective ?? null)
      : null,
  );
  const inspection = useAppStore((state) => selectErrandInspection(state, sessionId));
  const connectionStatus = useAppStore((state) => state.connection.status);
  const connectionGeneration = useAppStore((state) => state.connection.generation ?? 0);
  const readScopeIdentity = useAppStore(stateReadScopeIdentity);
  const observationRevision = useAppStore((state) =>
    taskId ? (state.errands.browserRevisionByTaskId[taskId] ?? 0) : 0,
  );
  const taskResetEpoch = useAppStore((state) =>
    taskId ? (state.errands.resetEpochByTaskId[taskId] ?? 0) : 0,
  );
  const parentId = errand?.parent_session_id || session?.parent_session_id || null;
  const parentSessionId = useAppStore((state) => {
    const parent = parentId ? state.taskSessions.items[parentId] : null;
    return parent?.task_id === taskId && parent.role !== "errand" ? parentId : null;
  });
  const parentTitle = useAppStore((state) =>
    parentSessionId ? selectSessionTabTitle(state, parentSessionId) : null,
  );
  const isErrand = session?.role === "errand" || Boolean(errand) || Boolean(inspection);

  useLayoutEffect(() => {
    if (!sessionId || !isErrand) return;
    const detailOwner = `${ownerId}:conversation`;
    store.getState().retainErrandDetail(detailOwner, sessionId);
    return () => store.getState().releaseErrandInspection(detailOwner);
  }, [isErrand, ownerId, readScopeIdentity, sessionId, store]);

  useErrandInspectionOwner({
    enabled,
    taskId,
    sessionId,
    isErrand,
    ownerId,
    store,
    connectionStatus,
    connectionGeneration,
    readScopeIdentity,
    taskResetEpoch,
    observationRevision,
  });

  return useMemo(() => {
    if (!sessionId || !taskId || !isErrand) return null;
    const browserErrand = errand ?? null;
    return {
      sessionId,
      taskId,
      errand: browserErrand,
      parentSessionId,
      parentTitle: parentTitle || t("task:errandParentFallback"),
      title: browserErrand?.title || session?.name || t("task:errandUntitled"),
      objective,
      workspacePath: getSessionWorkspacePath(session) ?? null,
      branch: session?.worktree_branch || null,
    };
  }, [errand, isErrand, objective, parentSessionId, parentTitle, session, sessionId, t, taskId]);
}
