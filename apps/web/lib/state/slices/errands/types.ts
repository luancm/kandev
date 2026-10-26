import type { Errand } from "@/lib/types/errands";

export type ErrandsSliceState = {
  errands: {
    /** Errands per task, keyed by errand ID (equal to the errand's session ID). */
    byTaskId: Record<string, Record<string, Errand>>;
    /** Tasks whose `errand.list` snapshot has been applied. */
    loadedByTaskId: Record<string, boolean>;
    /** Advances on every live upsert, so a list response can tell it raced live events. */
    liveRevisionByTaskId: Record<string, number>;
  };
};

export type ErrandsSliceActions = {
  /** Applies one `errand.updated` row; an update older than the stored row is ignored. */
  upsertErrand: (errand: Errand) => void;
  /**
   * Applies an `errand.list` snapshot. `requestRevision` is the task's live
   * revision when the request was sent: if live updates landed since, rows
   * missing from the snapshot are kept rather than dropped.
   */
  setTaskErrands: (taskId: string, errands: Errand[], requestRevision: number) => void;
  /** Drops every errand and load marker of a task, for example on task deletion. */
  removeTaskErrands: (taskId: string) => void;
};

export type ErrandsSlice = ErrandsSliceState & ErrandsSliceActions;
