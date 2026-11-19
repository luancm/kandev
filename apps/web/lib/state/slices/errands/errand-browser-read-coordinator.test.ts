import { describe, expect, it } from "vitest";
import {
  ErrandBrowserReadCoordinator,
  type ErrandBrowserReadKey,
} from "./errand-browser-read-coordinator";

const key: ErrandBrowserReadKey = {
  kind: "summary",
  readScopeIdentity: "api:user-a",
  taskId: "task-a",
  connectionGeneration: 3,
  taskResetEpoch: 2,
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function abortable<T>(signal: AbortSignal, pending: Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const onAbort = () => reject(new DOMException("Aborted", "AbortError"));
    if (signal.aborted) {
      onAbort();
      return;
    }
    signal.addEventListener("abort", onAbort, { once: true });
    pending.then(resolve, reject).finally(() => signal.removeEventListener("abort", onAbort));
  });
}

describe("ErrandBrowserReadCoordinator", () => {
  it("keeps a coalesced request alive when its initiating consumer releases", async () => {
    const coordinator = new ErrandBrowserReadCoordinator();
    const pending = deferred<string>();
    let signal!: AbortSignal;
    const load = (nextSignal: AbortSignal) => {
      signal = nextSignal;
      return abortable(nextSignal, pending.promise);
    };
    const first = coordinator.acquire(key, 0, (nextSignal) => load(nextSignal));
    const second = coordinator.acquire(key, 0, (nextSignal) => load(nextSignal));

    await Promise.resolve();
    first.release();
    expect(signal.aborted).toBe(false);
    pending.resolve("shared");
    await expect(second.promise).resolves.toBe("shared");
  });

  it("starts a new attempt after the final waiter aborts a request", async () => {
    const coordinator = new ErrandBrowserReadCoordinator();
    const old = deferred<string>();
    const fresh = deferred<string>();
    const first = coordinator.acquire(key, 0, (signal) => abortable(signal, old.promise));
    const oldRejected = expect(first.promise).rejects.toMatchObject({ name: "AbortError" });
    first.release();
    const replacement = coordinator.acquire(key, 0, (signal) => abortable(signal, fresh.promise));

    expect(replacement.attemptId).not.toBe(first.attemptId);
    fresh.resolve("fresh");
    old.resolve("obsolete");
    await expect(replacement.promise).resolves.toBe("fresh");
    await oldRejected;
  });

  it("replaces an active request after its observation revision changes", async () => {
    const coordinator = new ErrandBrowserReadCoordinator();
    const old = deferred<string>();
    const fresh = deferred<string>();
    const first = coordinator.acquire(key, 4, (signal) => abortable(signal, old.promise));
    const oldRejected = expect(first.promise).rejects.toMatchObject({ name: "AbortError" });
    const replacement = coordinator.acquire(key, 5, (signal) => abortable(signal, fresh.promise));

    expect(replacement.attemptId).not.toBe(first.attemptId);
    fresh.resolve("current");
    old.resolve("stale");
    await expect(replacement.promise).resolves.toBe("current");
    await oldRejected;
    first.release();
  });
});
