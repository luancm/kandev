import type {
  Errand,
  ErrandBrowserSummary,
  ErrandHistoryPage,
  ErrandInspection,
} from "@/lib/types/errands";

export type ErrandBrowserReadObservation = {
  connectionGeneration: number;
  readScopeIdentity: string;
  taskResetEpoch: number;
  observationRevision: number;
};

export type ErrandBrowserReadState = ErrandBrowserReadObservation & {
  status: "idle" | "loading" | "loaded" | "error";
  error?: unknown;
  attemptId?: string;
};

export type ErrandHistoryChain = {
  taskId: string;
  parentSessionId: string;
  pages: ErrandHistoryPage[];
  totalFinished: number;
  hasMore: boolean;
  nextCursor: string | null;
  rangeStart: number;
  status: "idle" | "loading" | "loaded" | "error";
  error?: unknown;
  stale: boolean;
  requestCursor?: string | null;
  attemptId?: string;
  observation?: ErrandBrowserReadObservation;
};

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
    /** Authority that owns all compact browser reads; never hydrated. */
    browserReadScopeIdentity?: string;
    /** Bounded compact reads used by browser Errands surfaces. */
    browserSummaryByTaskId: Record<string, ErrandBrowserSummary>;
    browserSummaryReadByTaskId: Record<string, ErrandBrowserReadState>;
    browserRevisionByTaskId: Record<string, number>;
    unreadSignatureByTaskId: Record<string, string>;
    historyByOwnerId: Record<string, ErrandHistoryChain>;
    /** Compact detail is retained only while a visible child reader owns it. */
    detailObservationRevisionBySessionId?: Record<string, number>;
    detailBySessionId: Record<string, ErrandInspection>;
    detailOwnersBySessionId: Record<string, Record<string, true>>;
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
  resetErrandReadScope: () => void;
  beginErrandBrowserSummaryRead: (
    taskId: string,
    observation: ErrandBrowserReadObservation,
    attemptId: string,
  ) => void;
  setErrandBrowserSummary: (
    taskId: string,
    summary: ErrandBrowserSummary,
    observation: ErrandBrowserReadObservation,
    attemptId: string,
  ) => void;
  failErrandBrowserSummaryRead: (
    taskId: string,
    observation: ErrandBrowserReadObservation,
    attemptId: string,
    error: unknown,
  ) => void;
  openErrandHistory: (ownerId: string, taskId: string, parentSessionId: string) => void;
  restartErrandHistory: (ownerId: string) => void;
  beginErrandHistoryRead: (
    ownerId: string,
    observation: ErrandBrowserReadObservation,
    cursor: string | null,
    attemptId: string,
  ) => void;
  setErrandHistoryPage: (
    ownerId: string,
    page: ErrandHistoryPage,
    observation: ErrandBrowserReadObservation,
    cursor: string | null,
    attemptId: string,
  ) => void;
  failErrandHistoryRead: (ownerId: string, attemptId: string, error: unknown) => void;
  closeErrandHistory: (ownerId: string) => void;
  markErrandHistoryStale: (taskId: string, parentSessionId?: string) => void;
  markErrandBrowserUnread: (taskId: string, unreadSessionIds: string[]) => void;
  advanceErrandBrowserRevision: (taskId: string, sessionId?: string) => void;
  removeErrandBrowserSession: (taskId: string, sessionId: string) => void;
  retainErrandDetail: (ownerId: string, sessionId: string) => void;
  retainErrandInspection: (ownerId: string, inspection: ErrandInspection) => void;
  releaseErrandInspection: (ownerId: string) => void;
};

export type ErrandsSlice = ErrandsSliceState & ErrandsSliceActions;
