import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import {
  InvalidErrandInspectionError,
  readErrandInspection,
} from "@/lib/state/slices/errands/errand-browser-read-coordinator";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";

export class TaskSessionAdmissionError extends Error {
  constructor(readonly reason: "unavailable" | "read_failed") {
    super();
  }
}

export async function assertRequestedSession(
  store: StoreApi<AppState>,
  identity: TaskNavigationIdentity,
  taskId: string,
  sessionId?: string,
) {
  if (!sessionId) return undefined;
  if (
    identity.allSessionsResponse.sessions.some(
      (session) =>
        session.id === sessionId && session.task_id === taskId && session.role !== "errand",
    )
  ) {
    return undefined;
  }
  const read = readErrandInspection(store, taskId, sessionId);
  try {
    const inspection = await read.promise;
    return {
      inspection,
      retain: (ownerId: string) => read.retain(ownerId, inspection),
      isCurrent: read.isCurrent,
    };
  } catch (error) {
    if (error instanceof Error && error.name === "AbortError") throw error;
    throw new TaskSessionAdmissionError(
      identity.sessionListUnavailable && !(error instanceof InvalidErrandInspectionError)
        ? "read_failed"
        : "unavailable",
    );
  } finally {
    read.release();
  }
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
