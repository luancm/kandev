import { isToolPayloadRemovedError } from "@/lib/utils/tool-payload-retention";
import { useCallback, useEffect, useRef, useState } from "react";
import { fetchErrandShellCommandOutput } from "@/lib/api/domains/errand-api";
import {
  fetchShellCommandOutput,
  type ShellCommandOutputSnapshot,
} from "@/lib/api/domains/session-api";
import { isTerminalToolCallStatus } from "@/lib/utils/tool-call-status";

const POLL_INTERVAL_MS = 1_000;
const MAX_RETRY_INTERVAL_MS = 5_000;
const MAX_CACHED_OUTPUTS = 200;

export type UseShellCommandOutputOptions = {
  sessionId: string;
  errandOwner?: { taskId: string; errandId: string; readScope: string };
  messageId: string;
  isOpen: boolean;
  messageStatus?: string;
};

export type UseShellCommandOutputResult = {
  snapshot: ShellCommandOutputSnapshot | null;
  isLoading: boolean;
  error: Error | null;
  retry: () => void;
};

type SharedRequest = {
  controller: AbortController;
  consumers: Set<symbol>;
  terminalProjection: boolean;
  promise: Promise<ShellCommandOutputSnapshot>;
};

type SharedOutput = {
  snapshot: ShellCommandOutputSnapshot | null;
  request: SharedRequest | null;
};

const outputCache = new Map<string, SharedOutput>();

function retryDelay(failureCount: number) {
  return Math.min(POLL_INTERVAL_MS * 2 ** Math.max(0, failureCount - 1), MAX_RETRY_INTERVAL_MS);
}

function asError(error: unknown) {
  return error instanceof Error ? error : new Error("Command output unavailable");
}

function touchOutput(outputKey: string, output: SharedOutput) {
  outputCache.delete(outputKey);
  outputCache.set(outputKey, output);
}

function pruneOutputCache() {
  if (outputCache.size <= MAX_CACHED_OUTPUTS) return;
  for (const [outputKey, output] of outputCache) {
    if (output.request) continue;
    outputCache.delete(outputKey);
    if (outputCache.size <= MAX_CACHED_OUTPUTS) return;
  }
}

function readCachedOutput(outputKey: string) {
  const output = outputCache.get(outputKey);
  if (output) touchOutput(outputKey, output);
  return output?.snapshot ?? null;
}

function requestSharedOutput(
  outputKey: string,
  load: (signal: AbortSignal) => Promise<ShellCommandOutputSnapshot>,
  consumer: symbol,
  terminalProjection: boolean,
) {
  const output = outputCache.get(outputKey) ?? { snapshot: null, request: null };
  touchOutput(outputKey, output);
  if (output.snapshot && isTerminalToolCallStatus(output.snapshot.status)) {
    return Promise.resolve(output.snapshot);
  }
  if (output.request) {
    if (!terminalProjection || output.request.terminalProjection) {
      output.request.consumers.add(consumer);
      return output.request.promise;
    }
    output.request.controller.abort();
    output.request = null;
  }

  const controller = new AbortController();
  const request: SharedRequest = {
    controller,
    consumers: new Set([consumer]),
    terminalProjection,
    promise: load(controller.signal)
      .then((snapshot) => {
        if (output.request === request) {
          output.snapshot = snapshot;
          touchOutput(outputKey, output);
        }
        return snapshot;
      })
      .finally(() => {
        if (output.request === request) output.request = null;
        pruneOutputCache();
      }),
  };
  output.request = request;
  pruneOutputCache();
  return request.promise;
}

function releaseSharedOutput(outputKey: string, consumer: symbol) {
  const output = outputCache.get(outputKey);
  const request = output?.request;
  if (!output || !request) return;
  request.consumers.delete(consumer);
  if (request.consumers.size > 0) return;
  output.request = null;
  request.controller.abort();
}

export function useShellCommandOutput({
  sessionId,
  errandOwner,
  messageId,
  isOpen,
  messageStatus,
}: UseShellCommandOutputOptions): UseShellCommandOutputResult {
  const taskId = errandOwner?.taskId;
  const errandId = errandOwner?.errandId;
  let outputKey = sessionId ? `${sessionId}:${messageId}` : "";
  if (taskId && errandId) {
    outputKey = JSON.stringify(["errand", taskId, errandId, errandOwner?.readScope, messageId]);
  }
  const [snapshot, setSnapshot] = useState<ShellCommandOutputSnapshot | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [retryVersion, setRetryVersion] = useState(0);
  const snapshotRef = useRef<ShellCommandOutputSnapshot | null>(null);
  const outputKeyRef = useRef("");
  const messageStatusRef = useRef(messageStatus);
  messageStatusRef.current = messageStatus;

  useEffect(() => {
    if (!isOpen || !outputKey || !messageId) {
      setIsLoading(false);
      return;
    }

    if (outputKeyRef.current !== outputKey) {
      outputKeyRef.current = outputKey;
      snapshotRef.current = readCachedOutput(outputKey);
      setSnapshot(snapshotRef.current);
      setError(null);
    }

    if (snapshotRef.current && isTerminalToolCallStatus(snapshotRef.current.status)) {
      setIsLoading(false);
      return;
    }
    const consumer = Symbol(outputKey);
    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let failureCount = 0;
    const load = (signal: AbortSignal) =>
      taskId && errandId
        ? fetchErrandShellCommandOutput(taskId, errandId, messageId, { init: { signal } })
        : fetchShellCommandOutput(sessionId, messageId, { init: { signal } });

    const requestSnapshot = async () => {
      if (!snapshotRef.current) setIsLoading(true);
      try {
        const nextSnapshot = await requestSharedOutput(
          outputKey,
          load,
          consumer,
          isTerminalToolCallStatus(messageStatusRef.current),
        );
        if (disposed) return;
        failureCount = 0;
        snapshotRef.current = nextSnapshot;
        setSnapshot(nextSnapshot);
        setError(null);
        setIsLoading(false);
        if (
          !isTerminalToolCallStatus(nextSnapshot.status) &&
          !isTerminalToolCallStatus(messageStatusRef.current)
        ) {
          timer = setTimeout(requestSnapshot, POLL_INTERVAL_MS);
        }
      } catch (requestError) {
        if (disposed) return;
        if (isToolPayloadRemovedError(requestError)) {
          snapshotRef.current = null;
          setSnapshot(null);
          setError(requestError as Error);
          setIsLoading(false);
          return;
        }
        failureCount += 1;
        setError(asError(requestError));
        setIsLoading(false);
        if (!isTerminalToolCallStatus(messageStatusRef.current)) {
          timer = setTimeout(requestSnapshot, retryDelay(failureCount));
        }
      }
    };

    void requestSnapshot();
    return () => {
      disposed = true;
      clearTimeout(timer);
      releaseSharedOutput(outputKey, consumer);
    };
  }, [errandId, isOpen, messageId, messageStatus, outputKey, retryVersion, sessionId, taskId]);

  const retry = useCallback(() => setRetryVersion((version) => version + 1), []);
  const currentTarget = outputKeyRef.current === outputKey;
  return {
    snapshot: currentTarget ? snapshot : null,
    isLoading,
    error: currentTarget ? error : null,
    retry,
  };
}
