import { useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { isErrandConversation } from "@/lib/task-conversation-navigation";
import { useTaskSessions } from "./use-task-sessions";

/** Normal conversations offered as explicit context sources in launch dialogs. */
export function useContextSessionOptions(taskId: string) {
  const { t } = useTranslation();
  const { sessions, loadSessions } = useTaskSessions(taskId);
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const errands = useAppStore((state) => state.errands.byTaskId[taskId]);
  useEffect(() => {
    loadSessions(true);
    // The dialog refreshes its source list once on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return useMemo(
    () =>
      sessions
        .filter((session) => !isErrandConversation(session, errands))
        .sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())
        .map((session, index) => {
          const profile = profiles.find((candidate) => candidate.id === session.agent_profile_id);
          const parts = profile?.label.split(" \u2022 ");
          if (parts && parts.length > 1) parts.shift();
          const label = parts?.join(" \u2022 ") ?? t("task:panelAgent");
          return { id: session.id, label, index: index + 1, agentName: profile?.agent_name };
        }),
    [sessions, profiles, errands, t],
  );
}
