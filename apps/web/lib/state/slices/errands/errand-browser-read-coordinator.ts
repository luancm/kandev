import { generateUUID } from "@/lib/uuid";
import {
  getErrandBrowserSummary,
  getErrandHistoryPage,
  inspectErrand,
} from "@/lib/api/domains/errand-api";
import { storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { AppState } from "@/lib/state/app-state-types";
import type { ErrandInspection } from "@/lib/types/errands";
import type { StoreApi } from "zustand";
import { selectErrandInspection } from "./selectors";

export type ErrandBrowserReadKey = {
  kind: "summary" | "history" | "inspection";
  readScopeIdentity: string;
  taskId: string;
  parentSessionId?: string;
  childSessionId?: string;
  cursor?: string | null;
  limit?: number;
  connectionGeneration: number;
  taskResetEpoch: number;
  scopeResetEpoch?: number;
};

type Attempt<T> = {
  id: string;
  observationRevision: number;
  consumers: number;
  active: boolean;
  controller: AbortController;
  promise: Promise<T>;
};

export type ErrandBrowserReadLease<T> = {
  attemptId: string;
  promise: Promise<T>;
  release: () => void;
};

/** Coalesces one scoped browser read while keeping each consumer's lifetime independent. */
export class ErrandBrowserReadCoordinator {
  private attempts = new Map<string, Attempt<unknown>>();

  acquire<T>(
    key: ErrandBrowserReadKey,
    observationRevision: number,
    load: (signal: AbortSignal, attemptId: string) => Promise<T>,
  ): ErrandBrowserReadLease<T> {
    const encodedKey = JSON.stringify(key);
    let attempt = this.attempts.get(encodedKey) as Attempt<T> | undefined;
    if (attempt && attempt.observationRevision !== observationRevision) {
      this.attempts.delete(encodedKey);
      attempt.active = false;
      attempt.controller.abort();
      attempt = undefined;
    }
    if (!attempt) {
      const controller = new AbortController();
      const id = generateUUID();
      const created: Attempt<T> = {
        id,
        observationRevision,
        consumers: 0,
        active: true,
        controller,
        promise: Promise.resolve(undefined as T),
      };
      created.promise = Promise.resolve()
        .then(() => load(controller.signal, id))
        .then((value) => {
          if (!this.isCurrent(encodedKey, created)) throw abortError();
          return value;
        })
        .finally(() => {
          created.active = false;
          if (this.attempts.get(encodedKey) === created) this.attempts.delete(encodedKey);
        });
      attempt = created;
      this.attempts.set(encodedKey, created as Attempt<unknown>);
    }
    attempt.consumers += 1;

    let released = false;
    return {
      attemptId: attempt.id,
      promise: attempt.promise,
      release: () => {
        if (released) return;
        released = true;
        attempt!.consumers = Math.max(0, attempt!.consumers - 1);
        if (attempt!.consumers !== 0 || !attempt!.active) return;
        if (this.attempts.get(encodedKey) === attempt) this.attempts.delete(encodedKey);
        attempt!.active = false;
        attempt!.controller.abort();
      },
    };
  }

  dispose(): void {
    for (const attempt of this.attempts.values()) {
      attempt.active = false;
      attempt.controller.abort();
    }
    this.attempts.clear();
  }

  private isCurrent<T>(key: string, attempt: Attempt<T>): boolean {
    return (
      attempt.active && !attempt.controller.signal.aborted && this.attempts.get(key) === attempt
    );
  }
}

const coordinators = new WeakMap<object, ErrandBrowserReadCoordinator>();

export function errandBrowserReadCoordinator(store: object): ErrandBrowserReadCoordinator {
  let coordinator = coordinators.get(store);
  if (!coordinator) {
    coordinator = new ErrandBrowserReadCoordinator();
    coordinators.set(store, coordinator);
  }
  return coordinator;
}

function abortError(): DOMException {
  return new DOMException("The operation was aborted", "AbortError");
}

export function invalidateErrandBrowserReads(store: object): void {
  coordinators.get(store)?.dispose();
}

export function readErrandSummary(
  store: StoreApi<AppState>,
  taskId: string,
): (() => void) | undefined {
  const state = store.getState();
  if (state.connection.status !== "connected") return;
  const observation = {
    readScopeIdentity: storeReadScopeIdentity(store),
    connectionGeneration: state.connection.generation ?? 0,
    taskResetEpoch: state.errands.resetEpochByTaskId[taskId] ?? 0,
    observationRevision: state.errands.browserRevisionByTaskId[taskId] ?? 0,
  };
  const read = state.errands.browserSummaryReadByTaskId[taskId];
  if (
    read?.status === "loaded" &&
    read.connectionGeneration === observation.connectionGeneration &&
    read.readScopeIdentity === observation.readScopeIdentity &&
    read.taskResetEpoch === observation.taskResetEpoch &&
    read.observationRevision === observation.observationRevision
  )
    return;
  const lease = errandBrowserReadCoordinator(store).acquire(
    {
      kind: "summary",
      taskId,
      readScopeIdentity: observation.readScopeIdentity,
      connectionGeneration: observation.connectionGeneration,
      taskResetEpoch: observation.taskResetEpoch,
    },
    observation.observationRevision,
    (signal) => getErrandBrowserSummary(taskId, signal),
  );
  store.getState().beginErrandBrowserSummaryRead(taskId, observation, lease.attemptId);
  void lease.promise
    .then((result) => {
      store.getState().setErrandBrowserSummary(taskId, result, observation, lease.attemptId);
    })
    .catch((error: unknown) => {
      if (!(error instanceof Error && error.name === "AbortError"))
        store.getState().failErrandBrowserSummaryRead(taskId, observation, lease.attemptId, error);
    })
    .finally(lease.release);
  return lease.release;
}

/** A fulfilled inspection remains retainable only while its captured observation is current. */
export class InvalidErrandInspectionError extends Error {}

export function retainCurrentErrandInspection(
  store: StoreApi<AppState>,
  taskId: string,
  sessionId: string,
  ownerId: string,
): boolean {
  const state = store.getState();
  const existing = selectErrandInspection(state, sessionId);
  const revision = state.errands.browserRevisionByTaskId[taskId] ?? 0;
  const retainedRevision = state.errands.detailObservationRevisionBySessionId?.[sessionId];
  if (
    !existing ||
    !validInspection(existing, taskId, sessionId) ||
    (retainedRevision ?? revision) !== revision ||
    state.errands.removedSessionIdsByTaskId[taskId]?.[sessionId]
  )
    return false;
  state.retainErrandInspection(ownerId, existing);
  return true;
}

export function readErrandInspection(store: StoreApi<AppState>, taskId: string, sessionId: string) {
  const initial = store.getState();
  const scope = storeReadScopeIdentity(store);
  const generation = initial.connection.generation ?? 0;
  const epoch = initial.errands.resetEpochByTaskId[taskId] ?? 0;
  const scopeEpoch = initial.errands.scopeResetEpoch ?? 0;
  const revision = initial.errands.browserRevisionByTaskId[taskId] ?? 0;
  const isCurrent = () => {
    const state = store.getState();
    return (
      storeReadScopeIdentity(store) === scope &&
      (state.connection.generation ?? 0) === generation &&
      (state.errands.scopeResetEpoch ?? 0) === scopeEpoch &&
      (state.errands.resetEpochByTaskId[taskId] ?? 0) === epoch &&
      (state.errands.browserRevisionByTaskId[taskId] ?? 0) === revision &&
      !state.errands.removedSessionIdsByTaskId?.[taskId]?.[sessionId]
    );
  };
  const lease = errandBrowserReadCoordinator(store).acquire(
    {
      kind: "inspection",
      readScopeIdentity: scope,
      taskId,
      childSessionId: sessionId,
      connectionGeneration: generation,
      taskResetEpoch: epoch,
      scopeResetEpoch: scopeEpoch,
    },
    revision,
    (signal) => inspectErrand(taskId, sessionId, signal),
  );
  const promise = lease.promise.then((inspection) => {
    if (!isCurrent()) throw abortError();
    if (!validInspection(inspection, taskId, sessionId)) throw new InvalidErrandInspectionError();
    return inspection;
  });
  return {
    promise,
    release: lease.release,
    isCurrent,
    retain: (ownerId: string, inspection: ErrandInspection) => {
      if (!isCurrent() || !validInspection(inspection, taskId, sessionId)) return false;
      store.getState().retainErrandInspection(ownerId, inspection);
      return true;
    },
  };
}

function validInspection(inspection: ErrandInspection, taskId: string, sessionId: string): boolean {
  return (
    inspection.scope.task_id === taskId &&
    inspection.scope.child_session_id === sessionId &&
    inspection.errand.task_id === taskId &&
    inspection.errand.errand_id === sessionId &&
    (inspection.child_session_exists || inspection.owns_conversation === true)
  );
}

export class ErrandHistoryOwner {
  readonly id = generateUUID();
  private leases = new Map<string | null, ErrandBrowserReadLease<unknown>>();

  constructor(private readonly store: StoreApi<AppState>) {}

  private release() {
    for (const lease of this.leases.values()) lease.release();
    this.leases.clear();
  }

  dispose() {
    this.release();
    this.store.getState().closeErrandHistory(this.id);
  }

  close(taskId?: string, parentSessionId?: string) {
    const chain = this.store.getState().errands.historyByOwnerId[this.id];
    if (
      !chain ||
      (taskId && chain.taskId !== taskId) ||
      (parentSessionId && chain.parentSessionId !== parentSessionId)
    )
      return;
    this.release();
    this.store.getState().closeErrandHistory(this.id);
  }

  open(taskId: string, parentSessionId: string) {
    const old = this.store.getState().errands.historyByOwnerId[this.id];
    if (old && (old.taskId !== taskId || old.parentSessionId !== parentSessionId)) this.release();
    this.store.getState().openErrandHistory(this.id, taskId, parentSessionId);
    const chain = this.store.getState().errands.historyByOwnerId[this.id];
    if (!chain || chain.pages.length === 0 || chain.stale || chain.status === "idle")
      this.read(null);
  }

  restart(taskId: string, parentSessionId: string) {
    this.release();
    this.store.getState().openErrandHistory(this.id, taskId, parentSessionId);
    this.store.getState().restartErrandHistory(this.id);
    this.read(null);
  }

  more(taskId: string, parentSessionId: string) {
    const chain = this.store.getState().errands.historyByOwnerId[this.id];
    if (
      chain?.taskId === taskId &&
      chain.parentSessionId === parentSessionId &&
      !chain.stale &&
      chain.status !== "loading" &&
      chain.hasMore &&
      chain.nextCursor
    )
      this.read(chain.nextCursor);
  }

  retry(taskId: string, parentSessionId: string) {
    const chain = this.store.getState().errands.historyByOwnerId[this.id];
    if (
      chain?.taskId === taskId &&
      chain.parentSessionId === parentSessionId &&
      chain.status !== "loading"
    )
      this.read(chain.requestCursor ?? null);
  }

  private read(cursor: string | null) {
    const state = this.store.getState();
    const chain = state.errands.historyByOwnerId[this.id];
    if (!chain || state.connection.status !== "connected") return;
    const { taskId, parentSessionId } = chain;
    const observation = {
      readScopeIdentity: storeReadScopeIdentity(this.store),
      connectionGeneration: state.connection.generation ?? 0,
      taskResetEpoch: state.errands.resetEpochByTaskId[taskId] ?? 0,
      observationRevision: state.errands.browserRevisionByTaskId[taskId] ?? 0,
    };
    const lease = errandBrowserReadCoordinator(this.store).acquire(
      {
        kind: "history",
        taskId,
        parentSessionId,
        cursor,
        limit: 25,
        readScopeIdentity: observation.readScopeIdentity,
        connectionGeneration: observation.connectionGeneration,
        taskResetEpoch: observation.taskResetEpoch,
      },
      observation.observationRevision,
      (signal) =>
        getErrandHistoryPage(
          taskId,
          parentSessionId,
          { limit: 25, ...(cursor === null ? {} : { cursor }) },
          signal,
        ),
    );
    this.leases.get(cursor)?.release();
    this.leases.set(cursor, lease);
    this.store.getState().beginErrandHistoryRead(this.id, observation, cursor, lease.attemptId);
    void lease.promise
      .then((page) => {
        this.store
          .getState()
          .setErrandHistoryPage(this.id, page, observation, cursor, lease.attemptId);
      })
      .catch((error: unknown) => {
        if (!(error instanceof Error && error.name === "AbortError"))
          this.store.getState().failErrandHistoryRead(this.id, lease.attemptId, error);
      })
      .finally(() => {
        lease.release();
        if (this.leases.get(cursor) === lease) this.leases.delete(cursor);
      });
  }
}
