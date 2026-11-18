import { useCallback, useContext, useEffect, useId, useState, type ReactNode } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  errandParentSessionId,
  replaceTaskConversationUrl,
} from "@/lib/task-conversation-navigation";
import type { ErrandInspection } from "@/lib/types/errands";
import { OwnedErrandConversation } from "./chat/errands/owned-errand-conversation";
import {
  LinkedOwnedErrandContext,
  OwnedErrandNavigationContext,
} from "./chat/errands/owned-errand-navigation-context";

type ConversationRenderer = (sessionId: string | null, inactive: boolean) => ReactNode;

function errandMatchesParent(
  inspection: ErrandInspection,
  taskId: string | null,
  sessionId: string | null,
) {
  return (
    inspection.scope.task_id === taskId &&
    inspection.errand.parent_session_id === sessionId &&
    inspection.parent.available &&
    inspection.parent.id === sessionId
  );
}

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
  const store = useAppStoreApi();
  const ownerId = `errand-host-${useId()}`;
  const readScope = useAppStore(stateReadScopeIdentity);
  const linked = useContext(LinkedOwnedErrandContext);
  const [dismissedLink, setDismissedLink] = useState<ErrandInspection | null>(null);
  const [selection, setSelection] = useState<{
    taskId: string;
    parentId: string;
    errandId: string;
    readScope: string;
  } | null>(null);
  const active =
    selection?.taskId === taskId &&
    selection.parentId === sessionId &&
    selection.readScope === readScope
      ? selection
      : null;
  if (selection && !active) setSelection(null);
  const open = useCallback(
    (inspection: ErrandInspection) => {
      if (!taskId || !sessionId || !errandMatchesParent(inspection, taskId, sessionId))
        return false;
      const errand = inspection.errand;
      const state = store.getState();
      state.releaseErrandInspection(ownerId);
      state.retainErrandDetail(ownerId, errand.errand_id);
      replaceTaskConversationUrl(taskId, errand.errand_id);
      setSelection({ taskId, parentId: sessionId, errandId: errand.errand_id, readScope });
      return true;
    },
    [ownerId, readScope, sessionId, store, taskId],
  );
  useEffect(() => {
    if (!active) store.getState().releaseErrandInspection(ownerId);
  }, [active, ownerId, store]);
  useEffect(() => () => store.getState().releaseErrandInspection(ownerId), [ownerId, store]);
  const linkedHere =
    linked && linked !== dismissedLink && errandMatchesParent(linked, taskId, sessionId)
      ? linked
      : null;
  const errandId = active?.errandId ?? linkedHere?.errand.errand_id;
  const close = () => {
    setSelection(null);
    if (linkedHere) setDismissedLink(linkedHere);
    if (taskId && sessionId) replaceTaskConversationUrl(taskId, sessionId);
  };
  const parentSessionId = useAppStore((state) =>
    taskId
      ? errandParentSessionId(
          sessionId,
          state.taskSessionsByTask.itemsByTaskId[taskId] ?? [],
          state.errands.byTaskId[taskId],
          sessionId ? state.taskSessions.items[sessionId] : undefined,
        )
      : null,
  );
  return (
    <OwnedErrandNavigationContext.Provider value={open}>
      <TaskConversationView
        sessionId={sessionId}
        parentSessionId={parentSessionId}
        ownedConversation={
          taskId &&
          errandId && (
            <OwnedErrandConversation
              key={errandId}
              taskId={taskId}
              errandId={errandId}
              onClose={close}
            />
          )
        }
      >
        {children}
      </TaskConversationView>
    </OwnedErrandNavigationContext.Provider>
  );
}

/** A temporary child covers its retained parent without changing the parent's viewport. */
export function TaskConversationView({
  sessionId,
  parentSessionId,
  ownedConversation,
  children,
}: {
  sessionId: string | null;
  parentSessionId: string | null;
  ownedConversation?: ReactNode;
  children: ConversationRenderer;
}) {
  const temporary = parentSessionId !== null && parentSessionId !== sessionId;
  const hostId = temporary ? parentSessionId : sessionId;
  const covered = temporary || Boolean(ownedConversation);
  return (
    <div className="relative flex h-full min-h-0 flex-1 flex-col">
      <div
        key={hostId}
        className={`flex min-h-0 flex-1 flex-col${covered ? " invisible" : ""}`}
        inert={covered || undefined}
        aria-hidden={covered || undefined}
      >
        {children(hostId, covered)}
      </div>
      {covered && (
        <div
          key={ownedConversation ? "owned" : sessionId}
          className="absolute inset-0 flex min-h-0 flex-col"
        >
          {ownedConversation ?? children(sessionId, false)}
        </div>
      )}
    </div>
  );
}
