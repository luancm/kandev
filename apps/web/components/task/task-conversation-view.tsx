import type { ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { errandParentSessionId } from "@/lib/task-conversation-navigation";

type ConversationRenderer = (sessionId: string | null, inactive: boolean) => ReactNode;

/** Resolves the shared parent owner for each host's complete conversation region. */
export function TaskConversationPanel({
  taskId,
  sessionId,
  children,
}: {
  taskId: string | null;
  sessionId: string | null;
  children: ConversationRenderer;
}) {
  const parentSessionId = useAppStore((state) =>
    taskId
      ? errandParentSessionId(
          sessionId,
          state.taskSessionsByTask.itemsByTaskId[taskId] ?? [],
          state.errands.byTaskId[taskId],
        )
      : null,
  );
  return (
    <TaskConversationView sessionId={sessionId} parentSessionId={parentSessionId}>
      {children}
    </TaskConversationView>
  );
}

/** A temporary child covers its retained parent without changing the parent's viewport. */
export function TaskConversationView({
  sessionId,
  parentSessionId,
  children,
}: {
  sessionId: string | null;
  parentSessionId: string | null;
  children: ConversationRenderer;
}) {
  const temporary = parentSessionId !== null && parentSessionId !== sessionId;
  const hostId = temporary ? parentSessionId : sessionId;
  return (
    <div className="relative flex h-full min-h-0 flex-1 flex-col">
      <div
        key={hostId}
        className={`flex min-h-0 flex-1 flex-col${temporary ? " invisible" : ""}`}
        inert={temporary || undefined}
        aria-hidden={temporary || undefined}
      >
        {children(hostId, temporary)}
      </div>
      {temporary && (
        <div key={sessionId} className="absolute inset-0 flex min-h-0 flex-col">
          {children(sessionId, false)}
        </div>
      )}
    </div>
  );
}
