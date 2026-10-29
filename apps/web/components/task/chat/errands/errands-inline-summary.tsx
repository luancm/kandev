"use client";

import { useState } from "react";
import { IconChevronDown, IconShieldQuestion } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useTaskErrands } from "@/hooks/domains/session/use-task-errands";
import { useNow } from "@/hooks/use-now";
import type { TaskSessionRole } from "@/lib/types/errands";
import { ErrandsParentGroup, ErrandsStopAllButton } from "./errands-panel";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";
import { cn } from "@/lib/utils";

type SummaryProps = {
  taskId: string | null;
  sessionId: string | null;
  sessionRole?: TaskSessionRole;
};
const keepExpanded = () => {};

function CurrentErrandsSummary({ taskId }: { taskId: string }) {
  const { t } = useTranslation();
  const { errands, errandsByParent, liveCount } = useTaskErrands(taskId);
  const [open, setOpen] = useState(false);
  const nowMs = useNow(30_000);
  if (errands.length === 0) return null;
  const needsAttention = errands.some((errand) => errand.status === "needs_attention");
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      data-testid="errands-inline-summary"
      className="my-3 min-w-0 rounded-lg border border-border/60 bg-muted/20"
    >
      <div className="flex min-w-0 flex-wrap items-center gap-2 p-2">
        <CollapsibleTrigger asChild>
          <button
            type="button"
            data-testid="errands-inline-disclosure"
            className={cn(
              "flex min-h-7 min-w-0 flex-1 cursor-pointer flex-wrap items-center gap-2 rounded px-2 text-left text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              ERRAND_TOUCH_TARGET,
            )}
          >
            <IconChevronDown
              aria-hidden
              className={cn("h-3.5 w-3.5 shrink-0 transition-transform", !open && "-rotate-90")}
            />
            <span className="font-medium">{t("task:errandsPanelTitle")}</span>
            <span className="text-muted-foreground">
              {t("task:errandsPanelTotal", { count: errands.length })}
            </span>
            {liveCount > 0 && <span>{t("task:errandsChipRunning", { count: liveCount })}</span>}
            {needsAttention && (
              <span className="flex items-center gap-1 text-amber-600">
                <IconShieldQuestion aria-hidden className="h-3.5 w-3.5" />
                {t("task:errandStatusNeedsAttention")}
              </span>
            )}
          </button>
        </CollapsibleTrigger>
        {liveCount > 0 && (
          <div
            className={cn(
              "max-w-full shrink-0 [&>button]:h-7",
              "max-md:w-full max-md:[&>button]:min-h-11 [@media(pointer:coarse)]:[&>button]:min-h-11",
            )}
          >
            <ErrandsStopAllButton taskId={taskId} drawer={false} />
          </div>
        )}
      </div>
      <CollapsibleContent>
        <div className="flex min-w-0 flex-col gap-3 border-t p-3">
          {Object.entries(errandsByParent).map(([parentId, group]) => (
            <ErrandsParentGroup
              key={parentId}
              parentSessionId={parentId}
              errands={group}
              taskId={taskId}
              nowMs={nowMs}
              drawer={false}
              onClose={keepExpanded}
            />
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function ErrandsInlineSummary({ taskId, sessionId, sessionRole }: SummaryProps) {
  const enabled = useFeature("errands");
  if (!enabled || !taskId || !sessionId || sessionRole === "errand") return null;
  return <CurrentErrandsSummary key={`${taskId}:${sessionId}`} taskId={taskId} />;
}
