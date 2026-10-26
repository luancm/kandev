import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type { Errand } from "@/lib/types/errands";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { ErrandsSlice, ErrandsSliceState } from "./types";

export const defaultErrandsState: ErrandsSliceState = {
  errands: {
    byTaskId: {},
    loadedByTaskId: {},
    liveRevisionByTaskId: {},
  },
};

type ImmerSet = Parameters<StateCreator<AppState, [["zustand/immer", never]], [], ErrandsSlice>>[0];

/** Unparseable timestamps never count as older, so a malformed row cannot freeze an errand. */
function isOlderErrand(incoming: Errand, existing: Errand | undefined): boolean {
  if (!existing) return false;
  const incomingAt = parseStrictRfc3339Timestamp(incoming.updated_at);
  const existingAt = parseStrictRfc3339Timestamp(existing.updated_at);
  return incomingAt !== null && existingAt !== null && incomingAt < existingAt;
}

export const createErrandsSlice = (set: ImmerSet): ErrandsSlice => ({
  ...defaultErrandsState,

  upsertErrand: (errand) =>
    set((draft) => {
      const taskId = errand.task_id;
      const errands = (draft.errands.byTaskId[taskId] ??= {});
      if (isOlderErrand(errand, errands[errand.errand_id])) return;
      errands[errand.errand_id] = errand;
      draft.errands.liveRevisionByTaskId[taskId] =
        (draft.errands.liveRevisionByTaskId[taskId] ?? 0) + 1;
    }),

  setTaskErrands: (taskId, errands, requestRevision) =>
    set((draft) => {
      const current = (draft.errands.byTaskId[taskId] ??= {});
      const listed = errands.filter((errand) => errand.task_id === taskId);
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
    }),

  removeTaskErrands: (taskId) =>
    set((draft) => {
      delete draft.errands.byTaskId[taskId];
      delete draft.errands.loadedByTaskId[taskId];
      delete draft.errands.liveRevisionByTaskId[taskId];
    }),
});
