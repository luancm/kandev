import { useEffect, useRef, useState } from "react";
import { getErrandMessages } from "@/lib/api/domains/errand-api";
import type { Message } from "@/lib/types/http";
import type { MessageHistoryStatus } from "./use-message-fetch-state";

type Transcript = {
  key: string;
  messages: Message[];
  hasMore: boolean;
  status: MessageHistoryStatus;
  error?: unknown;
};

const emptyTranscript: Transcript = { key: "", messages: [], hasMore: false, status: "loading" };
type MessagePage = Awaited<ReturnType<typeof getErrandMessages>>;

function refreshTranscript(previous: Transcript, key: string, page: MessagePage): Transcript {
  const first = page.messages[0]?.id;
  const overlap = previous.key === key ? previous.messages.findIndex((m) => m.id === first) : -1;
  if (overlap > 0) {
    return {
      key,
      messages: [...previous.messages.slice(0, overlap), ...page.messages],
      hasMore: previous.hasMore,
      status: "ready",
    };
  }
  return { key, messages: page.messages, hasMore: page.has_more, status: "ready" };
}

/** Observes a visible Errand without hydrating an ordinary conversation. */
export function useOwnedErrandTranscript({
  taskId,
  errandId,
  readScope,
  revision,
  connected,
}: {
  taskId: string;
  errandId: string;
  readScope: string;
  revision: string | number;
  connected: boolean;
}) {
  const key = JSON.stringify([readScope, taskId, errandId]);
  const [snapshot, setSnapshot] = useState(emptyTranscript);
  const [retryCount, setRetryCount] = useState(0);
  const olderRead = useRef<AbortController | null>(null);
  const [olderRequest, setOlderRequest] = useState<{
    key: string;
    controller: AbortController;
  } | null>(null);
  const transcript = snapshot.key === key ? snapshot : emptyTranscript;

  useEffect(() => {
    if (!connected) return;
    const controller = new AbortController();
    void getErrandMessages(taskId, errandId, {}, controller.signal).then(
      (page) => {
        if (!controller.signal.aborted) {
          setSnapshot((previous) => refreshTranscript(previous, key, page));
        }
      },
      (error: unknown) => {
        if (!controller.signal.aborted) {
          setSnapshot((previous) => ({
            ...(previous.key === key ? previous : emptyTranscript),
            key,
            status: "unavailable",
            error,
          }));
        }
      },
    );
    return () => controller.abort();
  }, [connected, errandId, key, retryCount, revision, taskId]);

  useEffect(
    () => () => {
      olderRead.current?.abort();
      olderRead.current = null;
    },
    [connected, key],
  );

  async function loadMore() {
    const beforeMessageId = transcript.messages[0]?.id;
    if (!connected || !transcript.hasMore || !beforeMessageId || olderRead.current) return 0;
    const controller = new AbortController();
    olderRead.current = controller;
    setOlderRequest({ key, controller });
    try {
      const page = await getErrandMessages(
        taskId,
        errandId,
        { beforeMessageId },
        controller.signal,
      );
      if (controller.signal.aborted) return 0;
      setSnapshot((previous) =>
        previous.key === key && previous.messages[0]?.id === beforeMessageId
          ? {
              ...previous,
              messages: [...page.messages, ...previous.messages],
              hasMore: page.has_more,
            }
          : previous,
      );
      return page.messages.length;
    } catch {
      return 0;
    } finally {
      if (olderRead.current === controller) {
        olderRead.current = null;
        setOlderRequest(null);
      }
    }
  }

  return {
    ...transcript,
    loadMore,
    loadingMore: connected && olderRequest?.key === key && !olderRequest.controller.signal.aborted,
    retry: () => setRetryCount((count) => count + 1),
  };
}
