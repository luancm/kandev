"use client";

import { useTranslation } from "react-i18next";

const TAG_LABEL_KEYS = {
  errand: "chat:errandChip",
  finished: "chat:errandFinished",
} as const;

/** Small uppercase kind tag, matching the subagent type chip. */
export function ErrandTag({ kind }: { kind: keyof typeof TAG_LABEL_KEYS }) {
  const { t } = useTranslation();
  return (
    <span className="shrink-0 rounded bg-muted px-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
      {t(TAG_LABEL_KEYS[kind])}
    </span>
  );
}
