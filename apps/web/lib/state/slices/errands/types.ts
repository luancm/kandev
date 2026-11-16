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

export type ErrandsSliceState = {
  errands: {
    /** Live operational errands per task, keyed by their reserved session ID. */
    byTaskId: Record<string, Record<string, Errand>>;
    resetEpochByTaskId: Record<string, number>;
    scopeResetEpoch?: number;
    /** Removed session IDs block late browser reads and live errand events from restoring a row. */
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
  /** Drops a task's operational rows and browser ownership, for example on task deletion. */
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
