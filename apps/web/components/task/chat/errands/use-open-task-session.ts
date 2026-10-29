import { useCallback, useContext } from "react";
import {
  TaskSessionNavigationContext,
  type OpenTaskSessionTarget,
} from "./task-session-navigation-context";
export type { OpenTaskSessionTarget } from "./task-session-navigation-context";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { clearHiddenSessionPanel } from "../../dockview-hidden-session-panels";
import { markSessionTabUserActivationIntent } from "../../session-tab-activation-intent";

/**
 * Opens a session of the current task by explicit user navigation. The
 * desktop workbench adds or focuses its `session:<id>` panel, so other visible
 * panels keep their own composer target; phone and tablet layouts make it the
 * active session. Neither path changes the primary session.
 */
export function useOpenTaskSession(): (target: OpenTaskSessionTarget) => void {
  const ownerOpen = useContext(TaskSessionNavigationContext);
  const store = useAppStoreApi();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  return useCallback(
    ({ taskId, sessionId, title }: OpenTaskSessionTarget) => {
      if (ownerOpen) {
        ownerOpen({ taskId, sessionId, title });
        return;
      }
      const dock = useDockviewStore.getState();
      if (usesDesktopWorkbench && dock.api) {
        markSessionTabUserActivationIntent(sessionId);
        clearHiddenSessionPanel(dock.api, sessionId);
        addSessionPanel(dock.api, dock.centerGroupId, sessionId, title);
        return;
      }
      store.getState().setActiveSession(taskId, sessionId);
    },
    [store, usesDesktopWorkbench, ownerOpen],
  );
}
