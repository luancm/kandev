import type { Errand } from "@/lib/types/errands";

export type ErrandsListObservation = {
  connectionGeneration: number;
  readScopeIdentity: string;
  taskResetEpoch: number;
};

export type ErrandsSliceState = {
  errands: {
    /** Errands per task, keyed by errand ID (equal to the errand's session ID). */
    byTaskId: Record<string, Record<string, Errand>>;
    /** Tasks whose `errand.list` snapshot has been applied. */
    loadedByTaskId: Record<string, boolean>;
    loadedConnectionGenerationByTaskId: Record<string, number>;
    loadedReadScopeByTaskId: Record<string, string>;
    resetEpochByTaskId: Record<string, number>;
    /** Advances on every live upsert, so a list response can tell it raced live events. */
    liveRevisionByTaskId: Record<string, number>;
    /** Last accepted live revision per row; runtime-only and absent in legacy state. */
    liveRevisionByErrandIdByTaskId?: Record<string, Record<string, number>>;
    /** Removed session IDs block late list and live errand events from restoring a row. */
    removedSessionIdsByTaskId: Record<string, Record<string, true>>;
  };
};

export type ErrandsSliceActions = {
  /** Applies one `errand.updated` row; an update older than the stored row is ignored. */
  upsertErrand: (errand: Errand) => void;
  /**
   * Applies an `errand.list` snapshot. `requestRevision` is the task's live
   * revision when the request was sent: live rows observed since are kept
   * unless the list has a strictly newer timestamp. Missing rows are retained
   * when live updates raced the request.
   */
  setTaskErrands: (
    taskId: string,
    errands: Errand[],
    requestRevision: number,
    observation?: ErrandsListObservation,
  ) => void;
  /** Drops every errand and load marker of a task, for example on task deletion. */
  removeTaskErrands: (taskId: string) => void;
};

export type ErrandsSlice = ErrandsSliceState & ErrandsSliceActions;
