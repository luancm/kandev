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
import { inspectErrand } from "@/lib/api/domains/errand-api";
import { fetchTaskSession } from "@/lib/api";
import type { ErrandInspection } from "@/lib/types/errands";
import { errandBrowserReadCoordinator } from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import { storeReadScopeIdentity } from "@/lib/state/shared-resource-reads";

type AppStoreApi = ReturnType<typeof useAppStoreApi>;
type OpenSession = (target: OpenTaskSessionTarget) => Promise<boolean>;
type TaskSession = ReturnType<AppStoreApi["getState"]>["taskSessions"]["items"][string];
type TaskSessionList = ReturnType<
  AppStoreApi["getState"]
>["taskSessionsByTask"]["itemsByTaskId"][string];
type ErrandCache = ReturnType<AppStoreApi["getState"]>["errands"]["byTaskId"][string];

function isValidInspection(
  inspection: ErrandInspection,
  taskId: string,
  sessionId: string,
): boolean {
  return (
    inspection.scope.task_id === taskId &&
    inspection.scope.child_session_id === sessionId &&
    inspection.errand.errand_id === sessionId &&
    inspection.errand.task_id === taskId &&
    inspection.child_session_exists
  );
}

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
  usesDesktopWorkbench: boolean;
  isCurrent: () => boolean;
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
    usesDesktopWorkbench,
    isCurrent,
  } = args;
  try {
    const inspection = await readErrandInspection(store, taskId, sessionId);
    if (!isCurrent() || !isValidInspection(inspection, taskId, sessionId)) return false;
    store.getState().retainErrandInspection(ownerId, inspection);
    const child = await resolveErrandSession({
      store,
      taskId,
      sessionId,
      parentSessionId: inspection.errand.parent_session_id,
      sessions,
      isCurrent,
    });
    if (!child) return false;
    if (!isCurrent()) return false;
    return openInspectedErrand({
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
  } catch {
    return false;
  }
}

async function readErrandInspection(
  store: AppStoreApi,
  taskId: string,
  sessionId: string,
): Promise<ErrandInspection> {
  const current = store.getState();
  const observationRevision = current.errands.browserRevisionByTaskId[taskId] ?? 0;
  const lease = errandBrowserReadCoordinator(store).acquire(
    {
      kind: "inspection",
      readScopeIdentity: storeReadScopeIdentity(store),
      taskId,
      childSessionId: sessionId,
      connectionGeneration: current.connection.generation ?? 0,
      taskResetEpoch: current.errands.resetEpochByTaskId[taskId] ?? 0,
    },
    observationRevision,
    (signal) => inspectErrand(taskId, sessionId, signal),
  );
  try {
    return await lease.promise;
  } finally {
    lease.release();
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
}): Promise<TaskSession | undefined> {
  const { store, taskId, sessionId, parentSessionId, sessions, isCurrent } = args;
  const existing =
    store.getState().taskSessions.items[sessionId] ??
    sessions.find((item) => item.id === sessionId);
  if (isValidErrandSession(existing, taskId, sessionId, parentSessionId)) return existing;
  const response = await fetchTaskSession(sessionId, { cache: "no-store" });
  if (!isCurrent() || !isValidErrandSession(response.session, taskId, sessionId, parentSessionId)) {
    return undefined;
  }
  store.getState().setTaskSession(response.session);
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
  const store = useAppStoreApi();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const ownerId = `errand-open-${useId()}`;
  const requestAttemptRef = useRef(0);
  const navigationGenerationRef = useRef(0);
  const mountedRef = useRef(false);
  const ownerOpenRef = useRef(ownerOpen);
  if (ownerOpenRef.current !== ownerOpen) {
    ownerOpenRef.current = ownerOpen;
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
      const navigationGeneration = navigationGenerationRef.current;
      const state = store.getState();
      const selectedTaskId = state.tasks.activeTaskId;
      const selectedSessionId = state.tasks.activeSessionId;
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
          usesDesktopWorkbench,
          isCurrent,
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
