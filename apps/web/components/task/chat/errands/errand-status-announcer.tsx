"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { selectTaskErrands } from "@/lib/state/slices/errands/selectors";
import type { Errand } from "@/lib/types/errands";
import { errandStatusLabelKey } from "./errand-status";

/** Errands a chat view reports on: the errand it shows, or the errands its session started. */
export function errandsForView(errands: readonly Errand[], sessionId: string | null): Errand[] {
  if (!sessionId) return [];
  return errands.filter(
    (errand) => errand.errand_id === sessionId || errand.parent_session_id === sessionId,
  );
}

/**
 * Announcement text for status changes. The first observation only seeds the
 * baseline, and an errand that newly appears is not announced (its start card
 * shows it), so loading a transcript never reads out every errand.
 */
function useStatusChangeAnnouncement(errands: readonly Errand[], active: boolean): string {
  const { t } = useTranslation();
  const baseline = useRef<Map<string, string> | null>(null);
  const [message, setMessage] = useState("");
  useEffect(() => {
    const previous = baseline.current;
    baseline.current = new Map(
      errands.map((errand) => [
        errand.errand_id,
        JSON.stringify([errand.status, errand.attention_reason ?? null]),
      ]),
    );
    if (!previous || !active) return;
    const changed = errands.filter((errand) => {
      const before = previous.get(errand.errand_id);
      return (
        before !== undefined &&
        before !== JSON.stringify([errand.status, errand.attention_reason ?? null])
      );
    });
    if (changed.length === 0) return;
    setMessage(
      changed
        .map((errand) =>
          t("task:errandStatusAnnouncement", {
            title: errand.title,
            status: t(
              errandStatusLabelKey(errand.status, { attentionReason: errand.attention_reason }),
            ),
          }),
        )
        .join(" "),
    );
  }, [errands, active, t]);
  return message;
}

/** The one polite status region of a chat view for errand status changes. */
export function ErrandStatusAnnouncer({
  taskId,
  sessionId,
  active,
}: {
  taskId: string | null;
  sessionId: string | null;
  active: boolean;
}) {
  const enabled = useFeature("errands");
  const taskErrands = useAppStore((state) => selectTaskErrands(state, enabled ? taskId : null));
  const viewErrands = useMemo(
    () => errandsForView(taskErrands, sessionId),
    [taskErrands, sessionId],
  );
  const message = useStatusChangeAnnouncement(viewErrands, active);
  if (!enabled || !taskId) return null;
  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      data-testid="errand-status-announcer"
      className="sr-only"
    >
      {message}
    </div>
  );
}
