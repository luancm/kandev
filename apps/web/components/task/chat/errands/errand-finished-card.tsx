"use client";

import { useTranslation } from "react-i18next";
import { formatPromptDuration } from "@/lib/turn-duration";
import { ErrandOpenButton } from "./errand-open-button";
import type { ErrandDeliveryItem } from "./errand-parse";
import { ErrandSnapshotStatus } from "./errand-await";
import { ErrandHistoryDisclosure } from "./errand-history-disclosure";

/**
 * One delivered result. It renders the snapshot recorded on the parent
 * message, never the live errand row, so a later result cannot rewrite it.
 */
export function ErrandFinishedCard({
  item,
  taskId,
}: {
  item: ErrandDeliveryItem;
  taskId: string | null;
}) {
  const { t } = useTranslation();
  const title = item.title || t("task:errandUntitled");
  const duration =
    item.durationMs === null
      ? null
      : formatPromptDuration(item.durationMs / 1000, {
          s: t("task:durationUnitSeconds"),
          m: t("task:durationUnitMinutes"),
          h: t("task:durationUnitHours"),
        });
  return (
    <ErrandHistoryDisclosure
      testId="errand-finished-card"
      errandId={item.errandId}
      summary={
        <span className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-1 [overflow-wrap:anywhere]">
          <span>{t("chat:errandCompleted", { count: 1 })}</span>
          {duration !== null && (
            <>
              <span aria-hidden>·</span>
              <span data-testid="errand-finished-summary-duration">{duration}</span>
            </>
          )}
        </span>
      }
    >
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-medium text-foreground" title={title}>
          {title}
        </span>
        {(item.status || duration !== null) && (
          <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1">
            {item.status && (
              <ErrandSnapshotStatus
                status={item.status}
                cancelledByUser
                testId="errand-finished-status"
              />
            )}
            {item.status && duration !== null && <span aria-hidden>·</span>}
            {duration !== null && (
              <span data-testid="errand-finished-duration" className="text-muted-foreground">
                {duration}
              </span>
            )}
          </span>
        )}
        <ErrandOpenButton
          taskId={taskId}
          errandId={item.errandId}
          title={title}
          className="-my-0.5"
        />
      </div>
      {item.preview && (
        <p
          data-testid="errand-finished-preview"
          className="mt-1.5 whitespace-pre-wrap border-l-2 border-border/60 pl-2 text-foreground/80 [overflow-wrap:anywhere]"
        >
          {item.preview}
        </p>
      )}
    </ErrandHistoryDisclosure>
  );
}
