"use client";

import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { TaskStatusSummaryErrands } from "@/lib/types/task-status-summary";

export function KanbanCardErrands({ summary }: { summary?: TaskStatusSummaryErrands | null }) {
  const { t } = useTranslation();
  const enabled = useFeature("errands");
  if (!enabled || !summary || (summary.live <= 0 && !summary.waiting)) return null;

  const liveLabel = summary.live > 0 ? t("kanban:errandsLive", { count: summary.live }) : null;
  const waitingLabel = summary.waiting ? t("kanban:errandsWaiting") : null;
  const fullLabel = [liveLabel, waitingLabel].filter(Boolean).join(", ");

  return (
    <span
      data-testid="task-errands-summary"
      title={fullLabel}
      className="inline-flex min-w-0 max-w-full items-center gap-1 text-muted-foreground font-mono text-[10px]"
    >
      {liveLabel && (
        <span data-testid="task-errands-count" className="shrink-0">
          {liveLabel}
        </span>
      )}
      {waitingLabel && (
        <span data-testid="task-errands-waiting" title={waitingLabel} className="min-w-0 truncate">
          {waitingLabel}
        </span>
      )}
    </span>
  );
}
