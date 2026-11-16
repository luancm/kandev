"use client";

import { useMemo, type ReactNode } from "react";
import type { Message } from "@/lib/types/http";
import { stripSystemTags } from "@/lib/utils/system-tags";
import { ErrandFinishedCard } from "./errand-finished-card";
import { parseErrandDelivery } from "./errand-parse";

/** A parent user message that delivered at least one errand result. */
export function hasErrandDelivery(comment: Message): boolean {
  return comment.author_type === "user" && parseErrandDelivery(comment.metadata).length > 0;
}

/**
 * Whether the user typed anything visible. A notification turn carries only
 * the status block, which the backend strips from `content`; stripping again
 * keeps a status block that arrives unstripped from ever rendering as text.
 */
export function hasVisibleUserContent(comment: Message): boolean {
  const attachments = (comment.metadata as { attachments?: unknown } | undefined)?.attachments;
  if (Array.isArray(attachments) && attachments.length > 0) return true;
  return stripSystemTags(comment.content ?? "") !== "";
}

/** The user's own message (when there is one), then one Errand finished card per result. */
export function ErrandDeliveryMessage({
  comment,
  userMessage,
}: {
  comment: Message;
  userMessage: ReactNode;
}) {
  const items = useMemo(() => parseErrandDelivery(comment.metadata), [comment.metadata]);
  return (
    <div data-testid="errand-delivery-message" className="flex w-full min-w-0 flex-col gap-2">
      {userMessage}
      {items.map((item, index) => (
        <ErrandFinishedCard
          key={`${item.errandId}:${index}`}
          item={item}
          taskId={comment.task_id || null}
        />
      ))}
    </div>
  );
}
