"use client";

import { useAppStore } from "@/components/state-provider";
import { useOwnedErrandTranscript } from "@/hooks/domains/session/use-owned-errand-transcript";
import { useErrandTranscriptRevision } from "@/hooks/domains/session/use-errand-transcript-revision";
import { useProcessedMessages } from "@/hooks/use-processed-messages";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { selectErrandBrowserRowBySessionId } from "@/lib/state/slices/errands/selectors";
import { isLiveErrand } from "@/lib/types/errands";
import { MessageList } from "../message-list";
import { ErrandHeader } from "./errand-header";
import { ErrandStatusAnnouncer } from "./errand-status-announcer";

export function OwnedErrandConversation({
  taskId,
  errandId,
  onClose,
}: {
  taskId: string;
  errandId: string;
  onClose: () => void;
}) {
  const readScope = useAppStore(stateReadScopeIdentity);
  const connected = useAppStore((state) => state.connection.status === "connected");
  const generation = useAppStore((state) => state.connection.generation ?? 0);
  const errand = useAppStore((state) => selectErrandBrowserRowBySessionId(state, taskId, errandId));
  const transcriptRevision = useErrandTranscriptRevision(taskId, errandId, connected);
  const transcript = useOwnedErrandTranscript({
    taskId,
    errandId,
    readScope,
    connected,
    revision: `${generation}:${transcriptRevision}:${errand?.updated_at ?? ""}`,
  });
  const processed = useProcessedMessages(transcript.messages, taskId, null, null, {
    historyInitialized: transcript.status === "ready",
    hasOlderMessages: transcript.hasMore,
  });
  return (
    <section
      data-testid="errand-conversation"
      data-errand-id={errandId}
      className="flex min-h-0 min-w-0 flex-1 flex-col bg-background"
    >
      <ErrandHeader taskId={taskId} sessionId={errandId} onBack={onClose} />
      <ErrandStatusAnnouncer taskId={taskId} sessionId={errandId} active />
      <MessageList
        taskId={taskId}
        sessionId={null}
        items={processed.groupedItems}
        messages={transcript.messages}
        permissionsByToolCallId={processed.permissionsByToolCallId}
        childrenByParentToolCallId={processed.childrenByParentToolCallId}
        messagesLoading={transcript.status === "loading"}
        historyStatus={transcript.status}
        historyError={transcript.error}
        onRetryHistory={transcript.retry}
        isWorking={errand !== undefined && isLiveErrand(errand)}
        ownedPagination={{
          hasMore: transcript.hasMore,
          isLoadingMore: transcript.loadingMore,
          loadMore: transcript.loadMore,
        }}
      />
    </section>
  );
}
