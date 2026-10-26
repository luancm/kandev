import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { selectErrandBySessionId } from "@/lib/state/slices/errands/selectors";
import type { Errand } from "@/lib/types/errands";
import { selectSessionTabTitle } from "../../session-tab-title";

export type ErrandViewModel = {
  sessionId: string;
  taskId: string;
  /** Live errand row; absent until `errand.list` or `errand.updated` delivers it. */
  errand: Errand | null;
  parentSessionId: string | null;
  parentTitle: string;
  title: string;
  objective: string | null;
  workspacePath: string | null;
  branch: string | null;
};

/**
 * View model of the errand shown in a chat panel, or null when the panel's
 * session is not an errand or the errands flag is off.
 */
export function useErrandViewModel(
  sessionId: string | null | undefined,
  taskIdHint: string | null | undefined,
): ErrandViewModel | null {
  const { t } = useTranslation();
  const enabled = useFeature("errands");
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const taskId = session?.task_id ?? taskIdHint ?? null;
  const errand = useAppStore((state) =>
    enabled && taskId && sessionId ? selectErrandBySessionId(state, taskId, sessionId) : undefined,
  );
  const parentSessionId = errand?.parent_session_id || session?.parent_session_id || null;
  const parentTitle = useAppStore((state) =>
    parentSessionId ? selectSessionTabTitle(state, parentSessionId) : null,
  );
  const isErrand = session?.role === "errand" || Boolean(errand);
  return useMemo(() => {
    if (!enabled || !sessionId || !taskId || !isErrand) return null;
    return {
      sessionId,
      taskId,
      errand: errand ?? null,
      parentSessionId,
      parentTitle: parentTitle || t("task:errandParentFallback"),
      title: errand?.title || t("task:errandUntitled"),
      objective: errand?.objective || null,
      workspacePath: getSessionWorkspacePath(session) ?? null,
      branch: session?.worktree_branch || null,
    };
  }, [enabled, sessionId, taskId, isErrand, errand, parentSessionId, parentTitle, session, t]);
}
