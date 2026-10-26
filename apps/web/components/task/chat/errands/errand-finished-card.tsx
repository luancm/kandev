"use client";

import { useTranslation } from "react-i18next";
import { ErrandOpenButton } from "./errand-open-button";
import type { ErrandDeliveryItem } from "./errand-parse";
import { ErrandStatusLabel } from "./errand-status";
import { ErrandTag } from "./errand-tag";

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
  return (
    <section
      data-testid="errand-finished-card"
      data-errand-id={item.errandId}
      aria-label={t("chat:errandFinishedCardLabel", { title })}
      className="w-full min-w-0 rounded-md border border-border/60 bg-muted/20 px-2.5 py-2 text-xs"
    >
      <div className="flex min-w-0 items-center gap-2">
        <ErrandTag kind="finished" />
        <span className="min-w-0 flex-1 truncate font-medium text-foreground" title={title}>
          {title}
        </span>
        {item.status && (
          <ErrandStatusLabel status={item.status} cancelledByUser testId="errand-finished-status" />
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
          className="mt-1.5 line-clamp-3 whitespace-pre-wrap border-l-2 border-border/60 pl-2 text-foreground/80 [overflow-wrap:anywhere]"
        >
          {item.preview}
        </p>
      )}
    </section>
  );
}
