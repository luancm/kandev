"use client";

import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { TaskStatusSummaryErrands } from "@/lib/types/task-status-summary";

export function ThreadErrandsIndicator({ summary }: { summary?: TaskStatusSummaryErrands | null }) {
  const { t } = useTranslation();
  const enabled = useFeature("errands");
  if (!enabled || !summary || (summary.live <= 0 && !summary.waiting)) return null;
  const label = [
    summary.live > 0 ? t("threads:errandsLive", { count: summary.live }) : null,
    summary.waiting ? t("threads:errandsWaiting") : null,
  ]
    .filter(Boolean)
    .join(", ");
  return (
    <span
      data-testid="thread-errands-summary"
      title={label}
      className="min-w-0 truncate text-xs text-muted-foreground"
    >
      {label}
    </span>
  );
}
