import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { clearHiddenSessionPanel } from "../../dockview-hidden-session-panels";
import { markSessionTabUserActivationIntent } from "../../session-tab-activation-intent";

export type OpenTaskSessionTarget = {
  taskId: string;
  sessionId: string;
  /** Initial tab title; the session tab keeps it in sync afterwards. */
  title: string;
};

/**
 * Opens a session of the current task by explicit user navigation. The
 * desktop workbench adds or focuses its `session:<id>` panel, so other visible
 * panels keep their own composer target; phone and tablet layouts make it the
 * active session. Neither path changes the primary session.
 */
export function useOpenTaskSession(): (target: OpenTaskSessionTarget) => void {
  const store = useAppStoreApi();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  return useCallback(
    ({ taskId, sessionId, title }: OpenTaskSessionTarget) => {
      const dock = useDockviewStore.getState();
      if (usesDesktopWorkbench && dock.api) {
        markSessionTabUserActivationIntent(sessionId);
        clearHiddenSessionPanel(dock.api, sessionId);
        addSessionPanel(dock.api, dock.centerGroupId, sessionId, title);
        return;
      }
      store.getState().setActiveSession(taskId, sessionId);
    },
    [store, usesDesktopWorkbench],
  );
}
