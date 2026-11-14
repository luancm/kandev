import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import { inspectErrand } from "@/lib/api/domains/errand-api";
import type { ErrandInspection } from "@/lib/types/errands";

export class TaskSessionAdmissionError extends Error {
  constructor(readonly reason: "unavailable" | "read_failed") {
    super();
  }
}

export async function assertRequestedSession(
  identity: TaskNavigationIdentity,
  taskId: string,
  sessionId?: string,
): Promise<ErrandInspection | undefined> {
  if (!sessionId) return undefined;
  if (
    identity.allSessionsResponse.sessions.some(
      (session) =>
        session.id === sessionId && session.task_id === taskId && session.role !== "errand",
    )
  ) {
    return undefined;
  }
  let inspection: ErrandInspection;
  try {
    inspection = await inspectErrand(taskId, sessionId);
  } catch {
    throw new TaskSessionAdmissionError(
      identity.sessionListUnavailable ? "read_failed" : "unavailable",
    );
  }
  if (
    inspection.scope.task_id !== taskId ||
    inspection.scope.child_session_id !== sessionId ||
    inspection.errand.task_id !== taskId ||
    inspection.errand.errand_id !== sessionId ||
    !inspection.child_session_exists
  ) {
    throw new TaskSessionAdmissionError("unavailable");
  }
  return inspection;
}

export function TaskSessionUnavailable({ retry }: { retry?: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex h-full flex-col items-center justify-center gap-3 p-4"
      data-testid="task-session-unavailable"
    >
      <p role="alert" className="text-sm text-muted-foreground">
        {t(retry ? "task:sessionHistoryUnavailable" : "task:workflowMovePreviewUnknownSession")}
      </p>
      {retry && (
        <Button variant="outline" onClick={retry}>
          {t("task:retry")}
        </Button>
      )}
    </div>
  );
}
