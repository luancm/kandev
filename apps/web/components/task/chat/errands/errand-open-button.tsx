"use client";

import { IconArrowUpRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { useOpenTaskSession } from "./use-open-task-session";

/** Fine-pointer compact density, 44px hit area on phones and coarse pointers. */
export const ERRAND_TOUCH_TARGET =
  "max-md:min-h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11";

/** **Open** for one errand; a queued errand has no session to open yet. */
export function ErrandOpenButton({
  taskId,
  errandId,
  title,
  queued = false,
  className,
  testId,
}: {
  taskId: string | null;
  errandId: string;
  title: string;
  queued?: boolean;
  className?: string;
  testId?: string;
}) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      disabled={!taskId || queued}
      aria-label={t("chat:errandOpenAria", { title })}
      data-testid={testId ?? `errand-open-${errandId}`}
      onClick={() => {
        if (taskId) openSession({ taskId, sessionId: errandId, title });
      }}
      className={cn(
        "h-6 shrink-0 cursor-pointer gap-1 px-2 text-[11px]",
        ERRAND_TOUCH_TARGET,
        className,
      )}
    >
      {t("chat:errandOpen")}
      <IconArrowUpRight aria-hidden className="h-3 w-3" />
    </Button>
  );
}
