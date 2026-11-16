import { useEffect, useId, useLayoutEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  readErrandInspection,
  retainCurrentErrandInspection,
} from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import {
  selectErrandBySessionId,
  selectErrandBrowserRowBySessionId,
  selectErrandInspection,
} from "@/lib/state/slices/errands/selectors";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { ErrandBrowserRow } from "@/lib/types/errands";
import type { AppState } from "@/lib/state/store";
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
    if (retainCurrentErrandInspection(store, taskId, sessionId, ownerId)) {
      return () => store.getState().releaseErrandInspection(ownerId);
    }
    if (connectionStatus !== "connected") return;

    const lease = readErrandInspection(store, taskId, sessionId);
    void lease.promise
      .then((result) => {
        if (!released) lease.retain(ownerId, result);
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

type ErrandViewInputs = {
  enabled: boolean;
  session: AppState["taskSessions"]["items"][string] | null;
  taskId: string | null;
  errand: ErrandBrowserRow | undefined;
  objective: string | null;
  parentSessionId: string | null;
  parentTitle: string | null;
  isErrand: boolean;
  connectionStatus: string;
  connectionGeneration: number;
  readScopeIdentity: string;
  observationRevision: number;
  taskResetEpoch: number;
};

const EMPTY_INPUTS: ErrandViewInputs = {
  enabled: false,
  session: null,
  taskId: null,
  errand: undefined,
  objective: null,
  parentSessionId: null,
  parentTitle: null,
  isErrand: false,
  connectionStatus: "disconnected",
  connectionGeneration: 0,
  readScopeIdentity: "",
  observationRevision: 0,
  taskResetEpoch: 0,
};

function selectParentInputs(state: AppState, taskId: string | null, parentId: string | null) {
  const parent = parentId ? state.taskSessions.items[parentId] : null;
  const parentSessionId = parent?.task_id === taskId && parent.role !== "errand" ? parentId : null;
  return {
    parentSessionId,
    parentTitle: parentSessionId ? selectSessionTabTitle(state, parentSessionId) : null,
  };
}

function selectRecognizedErrandInputs(
  state: AppState,
  sessionId: string,
  inputs: Pick<ErrandViewInputs, "enabled" | "session" | "taskId" | "errand">,
): ErrandViewInputs {
  const { enabled, session, taskId, errand } = inputs;
  const parentId = errand?.parent_session_id || session?.parent_session_id || null;
  return {
    ...inputs,
    ...selectParentInputs(state, taskId, parentId),
    objective:
      enabled && taskId
        ? (selectErrandBySessionId(state, taskId, sessionId)?.objective ?? null)
        : null,
    isErrand: true,
    connectionStatus: state.connection.status,
    connectionGeneration: state.connection.generation ?? 0,
    readScopeIdentity: stateReadScopeIdentity(state),
    observationRevision: taskId ? (state.errands.browserRevisionByTaskId[taskId] ?? 0) : 0,
    taskResetEpoch: taskId ? (state.errands.resetEpochByTaskId[taskId] ?? 0) : 0,
  };
}

function selectErrandViewInputs(
  state: AppState,
  sessionId: string | null | undefined,
  taskIdHint: string | null | undefined,
): ErrandViewInputs {
  if (!sessionId) return EMPTY_INPUTS;
  const enabled = state.features.errands;
  const session = state.taskSessions.items[sessionId] ?? null;
  const taskId = session?.task_id ?? taskIdHint ?? null;
  const errand = enabled ? selectErrandBrowserRowBySessionId(state, taskId, sessionId) : undefined;
  const inspection = selectErrandInspection(state, sessionId);
  const isErrand = session?.role === "errand" || Boolean(errand) || Boolean(inspection);
  if (!isErrand) return EMPTY_INPUTS;
  return selectRecognizedErrandInputs(state, sessionId, { enabled, session, taskId, errand });
}

/** View model of the selected child conversation and its targeted inspection. */
export function useErrandViewModel(
  sessionId: string | null | undefined,
  taskIdHint: string | null | undefined,
): ErrandViewModel | null {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const ownerId = `errand-view-${useId()}`;
  const {
    enabled,
    session,
    taskId,
    errand,
    objective,
    parentSessionId,
    parentTitle,
    isErrand,
    connectionStatus,
    connectionGeneration,
    readScopeIdentity,
    observationRevision,
    taskResetEpoch,
  } = useAppStore(useShallow((state) => selectErrandViewInputs(state, sessionId, taskIdHint)));

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
