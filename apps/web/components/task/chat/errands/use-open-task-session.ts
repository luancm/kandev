import { useCallback, useContext, useEffect, useId, useRef } from "react";
import {
  TaskSessionNavigationContext,
  type OpenTaskSessionTarget,
} from "./task-session-navigation-context";
export type { OpenTaskSessionTarget } from "./task-session-navigation-context";
import {
  conversationTabSessionId,
  isErrandConversation,
  replaceTaskConversationUrl,
} from "@/lib/task-conversation-navigation";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { clearHiddenSessionPanel } from "../../dockview-hidden-session-panels";
import { markSessionTabUserActivationIntent } from "../../session-tab-activation-intent";
import { fetchTaskSession } from "@/lib/api";
import type { ErrandInspection } from "@/lib/types/errands";
import { readErrandInspection } from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import { OwnedErrandNavigationContext } from "./owned-errand-navigation-context";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;
type OpenSession = (target: OpenTaskSessionTarget) => Promise<boolean>;
type TaskSession = ReturnType<AppStoreApi["getState"]>["taskSessions"]["items"][string];
type TaskSessionList = ReturnType<
  AppStoreApi["getState"]
>["taskSessionsByTask"]["itemsByTaskId"][string];
type ErrandCache = ReturnType<AppStoreApi["getState"]>["errands"]["byTaskId"][string];

function focusParentSurface(hostId: string | null) {
  const dock = useDockviewStore.getState();
  if (!dock.api || !hostId) return;
  clearHiddenSessionPanel(dock.api, hostId);
  dock.api.getPanel?.(`session:${hostId}`)?.api.setActive();
}

async function inspectAndOpenErrand(args: {
  store: AppStoreApi;
  ownerId: string;
  taskId: string;
  sessionId: string;
  title: string;
  target: OpenTaskSessionTarget;
  sessions: TaskSessionList;
  errands: ErrandCache;
  ownerOpen: ((target: OpenTaskSessionTarget) => void) | null;
  openOwned: ((inspection: ErrandInspection) => boolean) | null;
  usesDesktopWorkbench: boolean;
  isCurrent: () => boolean;
  isLatest: () => boolean;
  onRetained: () => void;
  onTransfer: () => void;
}): Promise<boolean> {
  const {
    store,
    ownerId,
    taskId,
    sessionId,
    title,
    target,
    sessions,
    errands,
    ownerOpen,
    openOwned,
    usesDesktopWorkbench,
    isCurrent,
    isLatest,
    onRetained,
    onTransfer,
  } = args;
  const read = readErrandInspection(store, taskId, sessionId);
  const observationCurrent = () => isCurrent() && read.isCurrent();
  let opened = false;
  try {
    const inspection = await read.promise;
    if (!observationCurrent() || !read.retain(ownerId, inspection)) return false;
    onRetained();
    if (inspection.owns_conversation === true && openOwned) {
      opened = openOwned(inspection);
      if (opened) onTransfer();
      return opened;
    }
    const child = await resolveErrandSession({
      store,
      taskId,
      sessionId,
      parentSessionId: inspection.errand.parent_session_id,
      sessions,
      isCurrent: observationCurrent,
      publish: (publish) => read.publishSession(ownerId, inspection, publish),
    });
    if (!child) return false;
    if (!observationCurrent()) return false;
    onTransfer();
    opened = openInspectedErrand({
      store,
      target,
      taskId,
      sessionId,
      title,
      inspection,
      child,
      sessions,
      errands,
      ownerOpen,
      usesDesktopWorkbench,
    });
    return opened;
  } catch {
    return false;
  } finally {
    read.release();
    if (!opened && isLatest()) {
      onTransfer();
      store.getState().releaseErrandInspection(ownerId);
    }
  }
}

function isValidErrandSession(
  session: TaskSession | undefined,
  taskId: string,
  sessionId: string,
  parentSessionId: string,
): session is TaskSession {
  return Boolean(
    session &&
    session.id === sessionId &&
    session.task_id === taskId &&
    session.role === "errand" &&
    session.parent_session_id === parentSessionId,
  );
}

async function resolveErrandSession(args: {
  store: AppStoreApi;
  taskId: string;
  sessionId: string;
  parentSessionId: string;
  sessions: TaskSessionList;
  isCurrent: () => boolean;
  publish: (publish: () => void) => boolean;
}): Promise<TaskSession | undefined> {
  const { store, taskId, sessionId, parentSessionId, sessions, isCurrent, publish } = args;
  const existing =
    store.getState().taskSessions.items[sessionId] ??
    sessions.find((item) => item.id === sessionId);
  if (isValidErrandSession(existing, taskId, sessionId, parentSessionId)) return existing;
  const response = await fetchTaskSession(sessionId, { cache: "no-store" });
  if (!isCurrent() || !isValidErrandSession(response.session, taskId, sessionId, parentSessionId)) {
    return undefined;
  }
  if (!publish(() => store.getState().setTaskSession(response.session))) return undefined;
  return response.session;
}

function openInspectedErrand(args: {
  store: AppStoreApi;
  target: OpenTaskSessionTarget;
  taskId: string;
  sessionId: string;
  title: string;
  inspection: ErrandInspection;
  child: TaskSession;
  sessions: TaskSessionList;
  errands: ErrandCache;
  ownerOpen: ((target: OpenTaskSessionTarget) => void) | null;
  usesDesktopWorkbench: boolean;
}): boolean {
  const {
    store,
    target,
    taskId,
    sessionId,
    title,
    inspection,
    child,
    sessions,
    errands,
    ownerOpen,
    usesDesktopWorkbench,
  } = args;
  if (ownerOpen) {
    ownerOpen({
      ...target,
      taskId,
      sessionId,
      title,
      isErrand: true,
      errand: inspection.errand,
      inspection,
    });
    return true;
  }
  replaceTaskConversationUrl(taskId, sessionId);
  store.getState().setActiveSession(taskId, sessionId);
  const hostId = conversationTabSessionId(sessionId, sessions, errands, child);
  if (usesDesktopWorkbench) focusParentSurface(hostId);
  return true;
}

function openRequestIsCurrent(args: {
  store: AppStoreApi;
  mounted: boolean;
  requestAttempt: number;
  latestRequestAttempt: number;
  navigationGeneration: number;
  currentNavigationGeneration: number;
  capturedOwner: ((target: OpenTaskSessionTarget) => void) | null;
  currentOwner: ((target: OpenTaskSessionTarget) => void) | null;
  selectedTaskId: string | null;
  selectedSessionId: string | null;
}): boolean {
  const { store } = args;
  const state = store.getState();
  return (
    args.mounted &&
    args.requestAttempt === args.latestRequestAttempt &&
    args.navigationGeneration === args.currentNavigationGeneration &&
    args.currentOwner === args.capturedOwner &&
    state.tasks.activeTaskId === args.selectedTaskId &&
    state.tasks.activeSessionId === args.selectedSessionId
  );
}

function isErrandTarget(
  target: OpenTaskSessionTarget,
  session: TaskSession | undefined,
  errands: ErrandCache,
): boolean {
  return target.isErrand === true || Boolean(session && isErrandConversation(session, errands));
}

function openOrdinaryTarget(args: {
  target: OpenTaskSessionTarget;
  session: TaskSession | undefined;
  ownerOpen: ((target: OpenTaskSessionTarget) => void) | null;
  taskId: string;
  sessionId: string;
  title: string;
  store: AppStoreApi;
  usesDesktopWorkbench: boolean;
}): boolean {
  const { target, session, ownerOpen, taskId, sessionId, title, store, usesDesktopWorkbench } =
    args;
  if (ownerOpen) {
    ownerOpen(target);
    return true;
  }
  if (!session) return false;
  replaceTaskConversationUrl(taskId, sessionId);
  const dock = useDockviewStore.getState();
  if (usesDesktopWorkbench && dock.api) {
    store.getState().setActiveSession(taskId, sessionId);
    markSessionTabUserActivationIntent(sessionId);
    clearHiddenSessionPanel(dock.api, sessionId);
    addSessionPanel(dock.api, dock.centerGroupId, sessionId, title);
    return true;
  }
  store.getState().setActiveSession(taskId, sessionId);
  return true;
}

/** Select a conversation through its embedded owner or the task workbench, preserving normal tab ownership. */
export function useOpenTaskSession(): OpenSession {
  const ownerOpen = useContext(TaskSessionNavigationContext);
  const openOwned = useContext(OwnedErrandNavigationContext);
  const store = useAppStoreApi();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const ownerId = `errand-open-${useId()}`;
  const requestAttemptRef = useRef(0);
  const navigationGenerationRef = useRef(0);
  const mountedRef = useRef(false);
  const provisionalAttemptRef = useRef<number | null>(null);
  const ownerOpenRef = useRef(ownerOpen);
  const ownedOpenRef = useRef(openOwned);
  if (ownerOpenRef.current !== ownerOpen || ownedOpenRef.current !== openOwned) {
    ownerOpenRef.current = ownerOpen;
    ownedOpenRef.current = openOwned;
    navigationGenerationRef.current++;
  }

  useEffect(() => {
    mountedRef.current = true;
    const unsubscribe = store.subscribe((state, previousState) => {
      if (
        state.tasks.activeTaskId !== previousState.tasks.activeTaskId ||
        state.tasks.activeSessionId !== previousState.tasks.activeSessionId
      ) {
        navigationGenerationRef.current++;
        if (provisionalAttemptRef.current !== null) {
          provisionalAttemptRef.current = null;
          store.getState().releaseErrandInspection(ownerId);
        }
      }
    });
    return () => {
      mountedRef.current = false;
      navigationGenerationRef.current++;
      requestAttemptRef.current++;
      unsubscribe();
      store.getState().releaseErrandInspection(ownerId);
    };
  }, [ownerId, store]);

  return useCallback(
    async (target: OpenTaskSessionTarget) => {
      const { taskId, sessionId, title } = target;
      const requestAttempt = ++requestAttemptRef.current;
      provisionalAttemptRef.current = null;
      store.getState().releaseErrandInspection(ownerId);
      const navigationGeneration = navigationGenerationRef.current;
      const state = store.getState();
      const { activeTaskId: selectedTaskId, activeSessionId: selectedSessionId } = state.tasks;
      const capturedOwner = ownerOpenRef.current;
      const isCurrent = () =>
        openRequestIsCurrent({
          store,
          mounted: mountedRef.current,
          requestAttempt,
          latestRequestAttempt: requestAttemptRef.current,
          navigationGeneration,
          currentNavigationGeneration: navigationGenerationRef.current,
          capturedOwner,
          currentOwner: ownerOpenRef.current,
          selectedTaskId,
          selectedSessionId,
        });

      const sessions = state.taskSessionsByTask?.itemsByTaskId?.[taskId] ?? [];
      const errands = state.errands?.byTaskId?.[taskId];
      const session =
        state.taskSessions?.items?.[sessionId] ?? sessions.find((item) => item.id === sessionId);
      if (session && session.task_id !== taskId) return false;
      if (isErrandTarget(target, session, errands)) {
        return inspectAndOpenErrand({
          store,
          ownerId,
          taskId,
          sessionId,
          title,
          target,
          sessions,
          errands,
          ownerOpen: capturedOwner,
          openOwned: ownedOpenRef.current,
          usesDesktopWorkbench,
          isCurrent,
          isLatest: () => requestAttemptRef.current === requestAttempt,
          onRetained: () => (provisionalAttemptRef.current = requestAttempt),
          onTransfer: () => (provisionalAttemptRef.current = null),
        });
      }

      return openOrdinaryTarget({
        target,
        session,
        ownerOpen: capturedOwner,
        taskId,
        sessionId,
        title,
        store,
        usesDesktopWorkbench,
      });
    },
    [ownerId, store, usesDesktopWorkbench],
  );
}
