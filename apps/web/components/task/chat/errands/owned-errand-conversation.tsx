"use client";

import { useMemo } from "react";
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
import { OwnedErrandPermissionContext } from "./owned-errand-permission-context";
import { OwnedErrandContinuation } from "./owned-errand-continuation";

export function OwnedErrandConversation({
  taskId,
  errandId,
  onClose,
}: {
  taskId: string;
  errandId: string;
  onClose?: () => void;
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
  const permissionOwner = useMemo(() => ({ taskId, errandId }), [taskId, errandId]);
  return (
    <section
      data-testid="errand-conversation"
      data-errand-id={errandId}
      className="flex min-h-0 min-w-0 flex-1 flex-col bg-background"
    >
      <ErrandHeader taskId={taskId} errandId={errandId} onBack={onClose} />
      <ErrandStatusAnnouncer taskId={taskId} sessionId={errandId} active />
      <OwnedErrandPermissionContext.Provider value={permissionOwner}>
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
      </OwnedErrandPermissionContext.Provider>
      <OwnedErrandContinuation
        key={readScope}
        taskId={taskId}
        errandId={errandId}
        connected={connected}
        available={
          errand?.status === "done" ||
          errand?.status === "failed" ||
          errand?.attention_reason === "recovery_unresolved"
        }
        onSent={transcript.retry}
      />
    </section>
  );
}
