import { useCallback, useContext, useEffect, useId, useState, type ReactNode } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import { replaceTaskConversationUrl } from "@/lib/task-conversation-navigation";
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
      state.retainErrandInspection(ownerId, inspection);
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
  return (
    <OwnedErrandNavigationContext.Provider value={open}>
      <TaskConversationView
        sessionId={sessionId}
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

/** An Errand covers the retained parent without changing the parent's viewport. */
export function TaskConversationView({
  sessionId,
  ownedConversation,
  children,
}: {
  sessionId: string | null;
  ownedConversation?: ReactNode;
  children: ConversationRenderer;
}) {
  const covered = Boolean(ownedConversation);
  return (
    <div className="relative flex h-full min-h-0 flex-1 flex-col">
      <div
        key={sessionId}
        className={`flex min-h-0 flex-1 flex-col${covered ? " invisible" : ""}`}
        inert={covered || undefined}
        aria-hidden={covered || undefined}
      >
        {children(sessionId, covered)}
      </div>
      {covered && <div className="absolute inset-0 flex min-h-0 flex-col">{ownedConversation}</div>}
    </div>
  );
}
