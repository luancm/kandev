import { generateUUID } from "@/lib/uuid";

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
