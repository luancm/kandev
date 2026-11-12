"use client";

import {
  forwardRef,
  useId,
  useState,
  type ComponentPropsWithoutRef,
  type ForwardedRef,
} from "react";
import { IconAlertTriangle, IconShieldQuestion } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Drawer, DrawerContent, DrawerTrigger } from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useAppStore } from "@/components/state-provider";
import { isLiveErrand } from "@/lib/types/errands";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useTaskErrands } from "@/hooks/domains/session/use-task-errands";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { ErrandsPanel } from "./errands-panel";

type ChipTone = "live" | "attention" | "idle";
type ErrandsChipCountKey =
  | "task:errandsChipNeedsAttention"
  | "task:errandsChipRunning"
  | "task:errandsChipQueued";

function getStatusCountLabels(
  attentionCount: number,
  runningCount: number,
  queuedCount: number,
  format: (key: ErrandsChipCountKey, count: number) => string,
) {
  const labels: string[] = [];
  if (attentionCount > 0) labels.push(format("task:errandsChipNeedsAttention", attentionCount));
  if (runningCount > 0) labels.push(format("task:errandsChipRunning", runningCount));
  if (queuedCount > 0) labels.push(format("task:errandsChipQueued", queuedCount));
  return labels;
}

const TONE_CLASS: Record<ChipTone, string> = {
  live: "border-primary/30 bg-primary/10 text-primary hover:bg-primary/20",
  attention:
    "border-amber-500/40 bg-amber-500/10 text-amber-700 hover:bg-amber-500/20 dark:text-amber-300",
  idle: "border-border bg-muted/40 text-muted-foreground hover:bg-muted",
};

function ChipIndicator({ tone, recovery }: { tone: ChipTone; recovery: boolean }) {
  if (tone === "attention") {
    if (recovery) {
      return <IconAlertTriangle aria-hidden className="h-3.5 w-3.5 shrink-0 text-amber-500" />;
    }
    return <IconShieldQuestion aria-hidden className="h-3.5 w-3.5 shrink-0 text-amber-500" />;
  }
  if (tone === "idle") return null;
  return (
    <span aria-hidden="true" className="relative flex h-2 w-2 shrink-0">
      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
      <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
    </span>
  );
}

type ErrandsTriggerProps = {
  detailsId: string;
  label: string;
  ariaLabel: string;
  tone: ChipTone;
  recovery: boolean;
  open: boolean;
  usesDrawer: boolean;
} & Omit<ComponentPropsWithoutRef<"button">, "ref">;

const ErrandsTrigger = forwardRef<HTMLButtonElement, ErrandsTriggerProps>(function ErrandsTrigger(
  { detailsId, label, ariaLabel, tone, recovery, open, usesDrawer, className, ...buttonProps },
  ref: ForwardedRef<HTMLButtonElement>,
) {
  return (
    <button
      {...buttonProps}
      ref={ref}
      type="button"
      data-testid="errands-chip"
      data-tone={tone}
      aria-label={ariaLabel}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-controls={detailsId}
      className={cn(
        className,
        "inline-flex max-w-full cursor-pointer items-center justify-center gap-1.5 rounded-full border text-[11px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        TONE_CLASS[tone],
        usesDrawer ? "min-h-11 min-w-11 px-3 py-1" : "h-6 px-2.5",
      )}
    >
      <ChipIndicator tone={tone} recovery={recovery} />
      <span className="min-w-0 whitespace-normal text-center">{label}</span>
    </button>
  );
});

type ErrandsChipBodyProps = { taskId: string; sessionId?: string | null };

function ErrandsChipBody({ taskId, sessionId }: ErrandsChipBodyProps) {
  const { t, i18n } = useTranslation();
  const {
    errands: allErrands,
    errandsByParent,
    liveCount: taskLiveCount,
    isLoaded,
  } = useTaskErrands(taskId);
  const parentId = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.parent_session_id || sessionId : null,
  );
  const errands = parentId ? (errandsByParent[parentId] ?? []) : allErrands;
  const liveCount = errands.filter(isLiveErrand).length;
  const runningCount = errands.filter((errand) => errand.status === "running").length;
  const queuedCount = errands.filter((errand) => errand.status === "queued").length;
  const attentionCount = errands.filter((errand) => errand.status === "needs_attention").length;
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const usesDrawer = !isFinePointer || isMobile;
  const [open, setOpen] = useState(false);
  const detailsId = `errands-details-${useId()}`;
  if (errands.length === 0) return null;

  const recoveryNeedsAttention = errands.some(
    (errand) =>
      errand.status === "needs_attention" && errand.attention_reason === "recovery_unresolved",
  );
  const needsAttention = attentionCount > 0;
  let tone: ChipTone = "idle";
  if (needsAttention) tone = "attention";
  else if (liveCount > 0) tone = "live";
  const statusCounts = getStatusCountLabels(
    attentionCount,
    runningCount,
    queuedCount,
    (key, count) => t(key, { count }),
  );
  const label =
    statusCounts.length > 0
      ? t("task:errandsChipStatus", {
          counts: new Intl.ListFormat(
            i18n.resolvedLanguage === "pseudo" ? "en" : (i18n.resolvedLanguage ?? i18n.language),
            {
              style: "long",
              type: "conjunction",
            },
          ).format(statusCounts),
        })
      : t("task:errandsChipIdle", { count: errands.length });
  let ariaLabel = label;
  if (recoveryNeedsAttention) ariaLabel = t("task:errandsChipRecoveryAria", { label });
  const trigger = (
    <ErrandsTrigger
      detailsId={detailsId}
      label={label}
      ariaLabel={ariaLabel}
      tone={tone}
      recovery={recoveryNeedsAttention}
      open={open}
      usesDrawer={usesDrawer}
    />
  );
  const panel = (
    <ErrandsPanel
      taskId={taskId}
      errands={errands}
      errandsByParent={errandsByParent}
      liveCount={taskLiveCount}
      isLoaded={isLoaded}
      drawer={usesDrawer}
      onClose={() => setOpen(false)}
    />
  );

  if (usesDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          id={detailsId}
          data-testid="errands-drawer-content"
          className="max-h-[min(80dvh,calc(100dvh-env(safe-area-inset-bottom)))]"
        >
          {panel}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        id={detailsId}
        data-testid="errands-popover"
        side="top"
        align="start"
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        {panel}
      </PopoverContent>
    </Popover>
  );
}

/**
 * Status-bar chip of a task's errands. Renders nothing, and loads nothing,
 * unless the `errands` flag is on and the task has errands; its panel opens
 * as a popover with a fine pointer and as a bottom drawer otherwise.
 */
export function ErrandsChip({
  taskId,
  sessionId,
}: {
  taskId: string | null;
  sessionId?: string | null;
}) {
  const enabled = useFeature("errands");
  if (!enabled || !taskId) return null;
  return <ErrandsChipBody key={sessionId} taskId={taskId} sessionId={sessionId} />;
}
