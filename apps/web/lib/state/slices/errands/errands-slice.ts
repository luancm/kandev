import type { StateCreator } from "zustand";
import { stateReadScopeIdentity } from "../../shared-resource-reads";
import type { AppState } from "../../app-state-types";
import type { Errand } from "@/lib/types/errands";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { ErrandsListObservation, ErrandsSlice, ErrandsSliceState } from "./types";

export const defaultErrandsState: ErrandsSliceState = {
  errands: {
    byTaskId: {},
    loadedByTaskId: {},
    loadedConnectionGenerationByTaskId: {},
    loadedReadScopeByTaskId: {},
    resetEpochByTaskId: {},
    liveRevisionByTaskId: {},
    removedSessionIdsByTaskId: {},
  },
};

export function mergeErrandsHydrationState(
  initialErrands: ErrandsSliceState["errands"] | undefined,
): ErrandsSliceState["errands"] {
  return {
    ...defaultErrandsState.errands,
    ...initialErrands,
    loadedConnectionGenerationByTaskId: {},
    loadedReadScopeByTaskId: {},
    resetEpochByTaskId: {},
  };
}

type ImmerSet = Parameters<StateCreator<AppState, [["zustand/immer", never]], [], ErrandsSlice>>[0];

/** Unparseable timestamps never count as older, so a malformed row cannot freeze an errand. */
function isOlderErrand(incoming: Errand, existing: Errand | undefined): boolean {
  if (!existing) return false;
  const incomingAt = parseStrictRfc3339Timestamp(incoming.updated_at);
  const existingAt = parseStrictRfc3339Timestamp(existing.updated_at);
  return incomingAt !== null && existingAt !== null && incomingAt < existingAt;
}

function observationIsCurrent(
  state: AppState,
  taskId: string,
  observation: ErrandsListObservation,
): boolean {
  return (
    state.features.errands &&
    state.connection.status === "connected" &&
    (state.connection.generation ?? 0) === observation.connectionGeneration &&
    stateReadScopeIdentity(state) === observation.readScopeIdentity &&
    (state.errands.resetEpochByTaskId?.[taskId] ?? 0) === observation.taskResetEpoch
  );
}

export const createErrandsSlice = (set: ImmerSet): ErrandsSlice => ({
  ...defaultErrandsState,

  upsertErrand: (errand) =>
    set((draft) => {
      const taskId = errand.task_id;
      if (draft.errands.removedSessionIdsByTaskId[taskId]?.[errand.errand_id]) return;
      const errands = (draft.errands.byTaskId[taskId] ??= {});
      if (isOlderErrand(errand, errands[errand.errand_id])) return;
      errands[errand.errand_id] = errand;
      draft.errands.liveRevisionByTaskId[taskId] =
        (draft.errands.liveRevisionByTaskId[taskId] ?? 0) + 1;
    }),

  setTaskErrands: (taskId, errands, requestRevision, observation) =>
    set((draft) => {
      if (observation && !observationIsCurrent(draft, taskId, observation)) return;
      const current = (draft.errands.byTaskId[taskId] ??= {});
      const removedSessionIds = draft.errands.removedSessionIdsByTaskId[taskId] ?? {};
      const listed = errands.filter(
        (errand) => errand.task_id === taskId && !removedSessionIds[errand.errand_id],
      );
      const racedLiveUpdates =
        (draft.errands.liveRevisionByTaskId[taskId] ?? 0) !== requestRevision;
      if (!racedLiveUpdates) {
        const listedIds = new Set(listed.map((errand) => errand.errand_id));
        for (const errandId of Object.keys(current)) {
          if (!listedIds.has(errandId)) delete current[errandId];
        }
      }
      for (const errand of listed) {
        if (!isOlderErrand(errand, current[errand.errand_id])) current[errand.errand_id] = errand;
      }
      draft.errands.loadedByTaskId[taskId] = true;
      if (observation) {
        (draft.errands.loadedConnectionGenerationByTaskId ??= {})[taskId] =
          observation.connectionGeneration;
        (draft.errands.loadedReadScopeByTaskId ??= {})[taskId] = observation.readScopeIdentity;
      }
    }),

  removeTaskErrands: (taskId) =>
    set((draft) => {
      (draft.errands.resetEpochByTaskId ??= {})[taskId] =
        (draft.errands.resetEpochByTaskId?.[taskId] ?? 0) + 1;
      delete draft.errands.loadedConnectionGenerationByTaskId?.[taskId];
      delete draft.errands.loadedReadScopeByTaskId?.[taskId];
      delete draft.errands.byTaskId[taskId];
      delete draft.errands.loadedByTaskId[taskId];
      delete draft.errands.liveRevisionByTaskId[taskId];
      delete draft.errands.removedSessionIdsByTaskId[taskId];
    }),
});
