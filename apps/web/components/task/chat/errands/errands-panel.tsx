"use client";

import { IconLoader2, IconPlayerStop, IconX } from "@tabler/icons-react";
import { useCallback, useId, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { DrawerClose, DrawerDescription, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useToast } from "@/components/toast-provider";
import { useErrandActions } from "@/hooks/domains/session/use-errand-actions";
import { useNow } from "@/hooks/use-now";
import type {
  ErrandBrowserCounts,
  ErrandBrowserRow,
  ErrandBrowserSummary,
} from "@/lib/types/errands";
import { orderErrandPanel } from "@/lib/errand-panel-order";
import { cn } from "@/lib/utils";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";
import { ErrandsPanelRow } from "./errands-panel-row";
import { ErrandParentHeading, ErrandsHistoryGroup } from "./errands-history-disclosure";

const CLOCK_TICK_MS = 30_000;

export type ErrandsPanelProps = {
  taskId: string;
  outstanding: ErrandBrowserRow[];
  summary: ErrandBrowserSummary | undefined;
  counts: ErrandBrowserCounts;
  taskCounts: ErrandBrowserCounts;
  parentSessionId: string | null;
  summaryStatus: "idle" | "loading" | "loaded" | "error";
  drawer: boolean;
  /** Closes the surface hosting the panel. */
  onClose: () => void;
  onRetrySummary: () => void;
};

function ErrandRows({
  parentSessionId,
  errands,
  taskId,
  nowMs,
  drawer,
  onClose,
}: {
  parentSessionId: string;
  errands: ErrandBrowserRow[];
  taskId: string;
  nowMs: number;
  drawer: boolean;
  onClose: () => void;
}) {
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      data-testid="errands-panel-group"
      className="flex flex-col gap-1"
    >
      <ErrandParentHeading parentSessionId={parentSessionId} id={headingId} />
      <ul className="flex flex-col gap-1.5">
        {errands.map((errand) => (
          <ErrandsPanelRow
            key={errand.errand_id}
            errand={errand}
            taskId={taskId}
            nowMs={nowMs}
            drawer={drawer}
            onOpened={onClose}
          />
        ))}
      </ul>
    </section>
  );
}

export function ErrandsStopAllButton({ taskId, drawer }: { taskId: string; drawer: boolean }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { cancelAllErrands, isPending } = useErrandActions();
  const pending = isPending("cancel_all", taskId);
  return (
    <Button
      type="button"
      variant="destructive"
      size="sm"
      disabled={pending}
      data-testid="errands-stop-all"
      onClick={() => {
        cancelAllErrands(taskId).catch(() => {
          toast({ title: t("task:errandsStopAllFailed"), variant: "error" });
        });
      }}
      className={cn("w-full cursor-pointer justify-center gap-1 text-xs", drawer && "min-h-11")}
    >
      {pending ? (
        <IconLoader2 aria-hidden className="h-3.5 w-3.5 animate-spin" />
      ) : (
        <IconPlayerStop aria-hidden className="h-3.5 w-3.5" />
      )}
      {t("task:errandsStopAll")}
    </Button>
  );
}

function SummaryMessage({
  status,
  onRetry,
  drawer,
}: {
  status: ErrandsPanelProps["summaryStatus"];
  onRetry: () => void;
  drawer: boolean;
}) {
  const { t } = useTranslation();
  if (status === "loading" || status === "idle") {
    return (
      <p
        role="status"
        data-testid="errands-panel-loading"
        className="flex items-center gap-1.5 px-0.5 text-[11px] text-muted-foreground"
      >
        <IconLoader2 aria-hidden className="h-3 w-3 animate-spin" />
        {t("task:errandsPanelLoading")}
      </p>
    );
  }
  if (status !== "error") return null;
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p role="alert" className="text-[11px] text-destructive">
        {t("task:errandsPanelFailed")}
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRetry}
        className={cn(drawer && ERRAND_TOUCH_TARGET)}
      >
        {t("task:errandsHistoryRetry")}
      </Button>
    </div>
  );
}

function PanelCountIndicators({ counts }: { counts: ErrandBrowserCounts }) {
  const { t } = useTranslation();
  return (
    <ul
      data-testid="errands-panel-counts"
      aria-label={t("task:errandsPanelTitle")}
      className="flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-muted-foreground"
    >
      <li>{t("task:errandsPanelCapacity", { live: counts.live, capacity: counts.capacity })}</li>
      {counts.pending_results > 0 && (
        <li>{t("task:errandsPanelPendingResults", { count: counts.pending_results })}</li>
      )}
      {counts.unread > 0 && <li>{t("task:errandsPanelUnread", { count: counts.unread })}</li>}
    </ul>
  );
}

function PanelList(props: ErrandsPanelProps & { nowMs: number }) {
  const { t } = useTranslation();
  const { outstanding, summary, taskId, nowMs, drawer, onClose, summaryStatus, onRetrySummary } =
    props;
  const historyOwnerId = `errand-history-${useId()}`;
  const listRef = useRef<HTMLDivElement>(null);
  const resetScrollToTop = useCallback(() => {
    if (listRef.current) listRef.current.scrollTop = 0;
  }, []);
  const ordered = orderErrandPanel(outstanding);
  const parentCounts = summary?.parent_counts ?? [];
  const parentIds = new Set([
    ...parentCounts.map((item) => item.parent_session_id),
    ...outstanding.map((item) => item.parent_session_id),
  ]);
  const finishedParents = parentCounts.filter(
    (item) =>
      parentIds.has(item.parent_session_id) &&
      (!props.parentSessionId || item.parent_session_id === props.parentSessionId),
  );
  return (
    <div
      data-testid="errands-panel-list"
      ref={listRef}
      data-vaul-no-drag={drawer ? true : undefined}
      className={cn(
        "flex min-h-0 flex-col gap-3 overflow-y-auto overscroll-contain",
        drawer ? "flex-1 px-4 pb-3" : "max-h-72 px-0.5",
      )}
    >
      <SummaryMessage status={summaryStatus} onRetry={onRetrySummary} drawer={drawer} />
      {ordered.active.length > 0 && (
        <section aria-label={t("task:errandsOutstanding")} className="flex flex-col gap-2">
          <h2 className="text-xs font-semibold text-muted-foreground">
            {t("task:errandsOutstanding")}
          </h2>
          {ordered.active.map(({ parentId, errands }) => (
            <ErrandRows
              key={parentId}
              parentSessionId={parentId}
              errands={errands}
              taskId={taskId}
              nowMs={nowMs}
              drawer={drawer}
              onClose={onClose}
            />
          ))}
        </section>
      )}
      {summaryStatus === "loaded" && outstanding.length === 0 && props.counts.outstanding === 0 && (
        <p
          data-testid="errands-outstanding-empty"
          className="px-0.5 text-[11px] text-muted-foreground"
        >
          {t("task:errandsOutstandingEmpty")}
        </p>
      )}
      {finishedParents.length > 0 && (
        <section aria-label={t("automations:completed")} className="flex flex-col gap-2">
          <h2 className="text-xs font-semibold text-muted-foreground">
            {t("automations:completed")}
          </h2>
          {finishedParents.map(({ parent_session_id: parentId, counts }) => (
            <ErrandsHistoryGroup
              key={parentId}
              parentSessionId={parentId}
              taskId={taskId}
              totalFinished={counts.finished}
              historyOwnerId={historyOwnerId}
              nowMs={nowMs}
              drawer={drawer}
              onClose={onClose}
              resetScrollToTop={resetScrollToTop}
            />
          ))}
        </section>
      )}
    </div>
  );
}

function DrawerPanelHeader({
  counts,
  hasSummary,
}: {
  counts: ErrandBrowserCounts;
  hasSummary: boolean;
}) {
  const { t } = useTranslation();
  return (
    <DrawerHeader
      data-testid="errands-panel-header"
      className="shrink-0 flex-row items-center justify-between gap-3 text-left"
    >
      <div className="min-w-0">
        <DrawerTitle>{t("task:errandsPanelTitle")}</DrawerTitle>
        <DrawerDescription>{t("task:errandsPanelDescription")}</DrawerDescription>
        {hasSummary && <PanelCountIndicators counts={counts} />}
      </div>
      <DrawerClose asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("task:errandsPanelClose")}
          className={ERRAND_TOUCH_TARGET}
        >
          <IconX aria-hidden="true" />
        </Button>
      </DrawerClose>
    </DrawerHeader>
  );
}

function PopoverPanelHeader({
  total,
  counts,
  hasSummary,
}: {
  total: number;
  counts: ErrandBrowserCounts;
  hasSummary: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 flex-col gap-1 border-b border-border pb-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-semibold text-foreground">{t("task:errandsPanelTitle")}</span>
        <Badge variant="outline" className="text-[10px]">
          {t("task:errandsPanelTotal", { count: total })}
        </Badge>
      </div>
      {hasSummary && <PanelCountIndicators counts={counts} />}
    </div>
  );
}

/** Errands use summary totals and fetch finished pages only when a disclosure opens. */
export function ErrandsPanel(props: ErrandsPanelProps) {
  const { taskId, counts, taskCounts, summary, drawer } = props;
  const nowMs = useNow(CLOCK_TICK_MS);
  const footer = taskCounts.live > 0 && <ErrandsStopAllButton taskId={taskId} drawer={drawer} />;
  const total = counts.outstanding + counts.finished;
  if (drawer) {
    return (
      <div data-testid="errands-panel" className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <DrawerPanelHeader counts={counts} hasSummary={Boolean(summary)} />
        <PanelList {...props} nowMs={nowMs} />
        {footer && (
          <div
            data-testid="errands-panel-footer"
            className="shrink-0 border-t border-border p-4 pb-[max(1rem,env(safe-area-inset-bottom))]"
          >
            {footer}
          </div>
        )}
      </div>
    );
  }
  return (
    <div
      data-testid="errands-panel"
      className="flex max-h-[min(28rem,70dvh)] min-h-0 flex-col gap-2"
    >
      <PopoverPanelHeader total={total} counts={counts} hasSummary={Boolean(summary)} />
      <PanelList {...props} nowMs={nowMs} />
      {footer && <div className="shrink-0 border-t border-border pt-2">{footer}</div>}
    </div>
  );
}
