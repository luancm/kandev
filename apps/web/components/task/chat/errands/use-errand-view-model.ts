import { useEffect, useId, useLayoutEffect } from "react";
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
  errandId: string;
  taskId: string;
  errand: ErrandBrowserRow | null;
  parentSessionId: string | null;
  parentTitle: string;
  title: string;
  objective: string | null;
  workspacePath: string | null;
  branch: string | null;
};

function selectErrandParent(state: AppState, taskId: string, errandId: string, parentId?: string) {
  const retained = selectErrandInspection(state, errandId);
  const inspection = retained?.scope.task_id === taskId ? retained : undefined;
  const candidate = parentId ? state.taskSessions.items[parentId] : undefined;
  const parent = candidate?.task_id === taskId ? candidate : undefined;
  return {
    parentSessionId: parent?.id ?? null,
    parentTitle: parent ? selectSessionTabTitle(state, parent.id) : inspection?.parent.title,
    workspacePath: getSessionWorkspacePath(parent) ?? null,
    branch: parent?.worktree_branch ?? null,
  };
}

function selectErrandViewInputs(state: AppState, taskId: string, errandId: string) {
  const enabled = state.features.errands;
  const errand = selectErrandBrowserRowBySessionId(state, taskId, errandId);
  return {
    ...selectErrandParent(state, taskId, errandId, errand?.parent_session_id),
    enabled,
    errand: enabled ? (errand ?? null) : null,
    title: errand?.title,
    objective: enabled
      ? (selectErrandBySessionId(state, taskId, errandId)?.objective ?? null)
      : null,
    connectionStatus: state.connection.status,
    connectionGeneration: state.connection.generation ?? 0,
    readScopeIdentity: stateReadScopeIdentity(state),
    observationRevision: state.errands.browserRevisionByTaskId[taskId] ?? 0,
    taskResetEpoch: state.errands.resetEpochByTaskId[taskId] ?? 0,
  };
}

/** Header data for an Errand-owned conversation and its authorized parent. */
export function useErrandViewModel(errandId: string, taskId: string): ErrandViewModel {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const ownerId = `errand-view-${useId()}`;
  const inputs = useAppStore(
    useShallow((state) => selectErrandViewInputs(state, taskId, errandId)),
  );
  const {
    enabled,
    connectionStatus,
    connectionGeneration,
    readScopeIdentity,
    observationRevision,
    taskResetEpoch,
  } = inputs;

  useLayoutEffect(() => {
    const detailOwner = `${ownerId}:conversation`;
    store.getState().retainErrandDetail(detailOwner, errandId);
    return () => store.getState().releaseErrandInspection(detailOwner);
  }, [errandId, ownerId, readScopeIdentity, store]);

  useEffect(() => {
    if (!enabled) return;
    if (retainCurrentErrandInspection(store, taskId, errandId, ownerId)) {
      return () => store.getState().releaseErrandInspection(ownerId);
    }
    if (connectionStatus !== "connected") return;
    const read = readErrandInspection(store, taskId, errandId);
    let active = true;
    void read.promise
      .then((inspection) => {
        if (active) read.retain(ownerId, inspection);
      })
      .catch(() => undefined)
      .finally(read.release);
    return () => {
      active = false;
      read.release();
      store.getState().releaseErrandInspection(ownerId);
    };
  }, [
    connectionGeneration,
    connectionStatus,
    enabled,
    errandId,
    observationRevision,
    ownerId,
    readScopeIdentity,
    store,
    taskId,
    taskResetEpoch,
  ]);

  return {
    errandId,
    taskId,
    errand: inputs.errand,
    parentSessionId: inputs.parentSessionId,
    parentTitle: inputs.parentTitle || t("task:errandParentFallback"),
    title: inputs.title || t("task:errandUntitled"),
    objective: inputs.objective,
    workspacePath: inputs.workspacePath,
    branch: inputs.branch,
  };
}
