"use client";

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { IconChevronRight, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { useId, useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useTaskErrands } from "@/hooks/domains/session/use-task-errands";
import type { ErrandHistoryChain } from "@/lib/state/slices/errands/types";
import type { ErrandBrowserRow } from "@/lib/types/errands";
import { cn } from "@/lib/utils";
import { selectSessionTabTitle } from "../../session-tab-title";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";
import { ErrandsPanelRow } from "./errands-panel-row";

export function ErrandParentHeading({
  parentSessionId,
  id,
}: {
  parentSessionId: string;
  id: string;
}) {
  const { t } = useTranslation();
  const title = useAppStore((state) => selectSessionTabTitle(state, parentSessionId));
  return (
    <h3 id={id} className="truncate px-0.5 text-[11px] font-medium text-muted-foreground">
      {title || t("task:errandParentFallback")}
    </h3>
  );
}

export function ErrandsHistoryGroup({
  parentSessionId,
  taskId,
  totalFinished,
  historyOwnerId,
  nowMs,
  drawer,
  onClose,
  resetScrollToTop,
}: {
  parentSessionId: string;
  taskId: string;
  totalFinished: number;
  historyOwnerId: string;
  nowMs: number;
  drawer: boolean;
  onClose: () => void;
  resetScrollToTop: () => void;
}) {
  return (
    <section data-testid="errands-finished-parent-group" className="flex min-w-0 flex-col gap-1">
      <HistoryRows
        taskId={taskId}
        parentSessionId={parentSessionId}
        historyOwnerId={historyOwnerId}
        totalFinished={totalFinished}
        nowMs={nowMs}
        drawer={drawer}
        onClose={onClose}
        resetScrollToTop={resetScrollToTop}
      />
    </section>
  );
}

function HistoryRows({
  taskId,
  parentSessionId,
  historyOwnerId,
  totalFinished,
  nowMs,
  drawer,
  onClose,
  resetScrollToTop,
}: {
  taskId: string;
  parentSessionId: string;
  historyOwnerId: string;
  totalFinished: number;
  nowMs: number;
  drawer: boolean;
  onClose: () => void;
  resetScrollToTop: () => void;
}) {
  const { t } = useTranslation();
  const parentHeadingId = useId();
  const historyState = useTaskErrands(taskId, { parentSessionId, historyOwnerId });
  const chain = historyState.history;
  const isActiveHistory = chain?.taskId === taskId && chain.parentSessionId === parentSessionId;
  const history = isActiveHistory ? chain : undefined;
  const historyRows = isActiveHistory ? historyState.historyRows : [];
  const rangeStart = history?.rangeStart ?? 0;
  const previousRangeStart = useRef<number | null>(null);

  useLayoutEffect(() => {
    if (previousRangeStart.current !== null && previousRangeStart.current > 0 && rangeStart === 0) {
      resetScrollToTop();
    }
    previousRangeStart.current = rangeStart;
  }, [rangeStart, resetScrollToTop]);

  return (
    <Collapsible
      open={Boolean(history)}
      onOpenChange={(open) => (open ? historyState.openHistory() : historyState.closeHistory())}
      data-testid={`errands-history-${parentSessionId}`}
      className="min-w-0"
    >
      <CollapsibleTrigger
        data-testid={`errands-history-toggle-${parentSessionId}`}
        aria-label={t("task:errandsHistoryToggle", { count: totalFinished })}
        className={cn(
          "group flex w-full cursor-pointer items-center gap-2 rounded py-1.5 text-left text-xs text-muted-foreground hover:bg-muted/30",
          ERRAND_TOUCH_TARGET,
        )}
      >
        <IconChevronRight
          aria-hidden
          className="h-3.5 w-3.5 shrink-0 group-data-[state=open]:rotate-90"
        />
        {t("automations:completed")} ({totalFinished})
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 pt-2">
        <ErrandParentHeading parentSessionId={parentSessionId} id={parentHeadingId} />
        <HistoryStaleNotice
          parentSessionId={parentSessionId}
          history={history}
          drawer={drawer}
          onRefresh={historyState.refreshHistory}
        />
        <HistoryRowList
          parentSessionId={parentSessionId}
          rows={historyRows}
          rangeStart={history?.rangeStart ?? 0}
          taskId={taskId}
          nowMs={nowMs}
          drawer={drawer}
          onClose={onClose}
        />
        <HistoryFeedback
          parentSessionId={parentSessionId}
          history={history}
          rowCount={historyRows.length}
          drawer={drawer}
          onRetry={historyState.retryHistory}
        />
        <HistoryRangeControls
          parentSessionId={parentSessionId}
          history={history}
          rowCount={historyRows.length}
          drawer={drawer}
          onStartAgain={historyState.startHistoryAgain}
          onLoadMore={historyState.loadMoreHistory}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function HistoryStaleNotice({
  parentSessionId,
  history,
  drawer,
  onRefresh,
}: {
  parentSessionId: string;
  history: ErrandHistoryChain | undefined;
  drawer: boolean;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  if (!history?.stale) return null;
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded border border-border/60 bg-muted/20 px-2 py-1.5">
      <p role="status" className="min-w-0 flex-1 text-[11px] text-muted-foreground">
        {t("task:errandsHistoryStale")}
      </p>
      <Button
        type="button"
        variant="outline"
        size="sm"
        data-testid={`errands-history-refresh-${parentSessionId}`}
        onClick={onRefresh}
        className={cn("min-h-8", drawer && ERRAND_TOUCH_TARGET)}
      >
        <IconRefresh aria-hidden className="h-3.5 w-3.5" />
        {t("task:errandsHistoryRefresh")}
      </Button>
    </div>
  );
}

function HistoryRowList({
  parentSessionId,
  rows,
  rangeStart,
  taskId,
  nowMs,
  drawer,
  onClose,
}: {
  parentSessionId: string;
  rows: ErrandBrowserRow[];
  rangeStart: number;
  taskId: string;
  nowMs: number;
  drawer: boolean;
  onClose: () => void;
}) {
  if (rows.length === 0) return null;
  return (
    <ul
      data-testid={`errands-history-rows-${parentSessionId}`}
      data-range-start={rangeStart}
      className="flex flex-col gap-1.5"
    >
      {rows.map((errand) => (
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
  );
}

function HistoryFeedback({
  parentSessionId,
  history,
  rowCount,
  drawer,
  onRetry,
}: {
  parentSessionId: string;
  history: ErrandHistoryChain | undefined;
  rowCount: number;
  drawer: boolean;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (history?.status === "loading") {
    return (
      <p
        role="status"
        className="flex items-center gap-1.5 px-0.5 text-[11px] text-muted-foreground"
      >
        <IconLoader2 aria-hidden className="h-3 w-3 animate-spin" />
        {t("task:errandsHistoryLoading")}
      </p>
    );
  }
  if (history?.status === "error") {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p role="alert" className="text-[11px] text-destructive">
          {t("task:errandsHistoryFailed")}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          data-testid={`errands-history-retry-${parentSessionId}`}
          onClick={onRetry}
          className={cn("min-h-8", drawer && ERRAND_TOUCH_TARGET)}
        >
          {t("task:errandsHistoryRetry")}
        </Button>
      </div>
    );
  }
  if (history?.status === "loaded" && rowCount === 0) {
    return (
      <p
        data-testid={`errands-history-empty-${parentSessionId}`}
        className="text-[11px] text-muted-foreground"
      >
        {t("task:errandsHistoryEmpty")}
      </p>
    );
  }
  return null;
}

function HistoryRangeControls({
  parentSessionId,
  history,
  rowCount,
  drawer,
  onStartAgain,
  onLoadMore,
}: {
  parentSessionId: string;
  history: ErrandHistoryChain | undefined;
  rowCount: number;
  drawer: boolean;
  onStartAgain: () => void;
  onLoadMore: () => void;
}) {
  const { t } = useTranslation();
  if (!history || rowCount === 0) return null;
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border/40 pt-2">
      <p
        data-testid={`errands-history-range-${parentSessionId}`}
        className="text-[11px] tabular-nums text-muted-foreground"
      >
        {t("task:errandsHistoryRange", {
          start: history.rangeStart + 1,
          end: history.rangeStart + rowCount,
          total: history.totalFinished,
        })}
      </p>
      <div className="flex flex-wrap gap-2">
        {history.rangeStart > 0 && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            data-testid={`errands-history-start-again-${parentSessionId}`}
            onClick={onStartAgain}
            className={cn("min-h-8", drawer && ERRAND_TOUCH_TARGET)}
          >
            {t("task:errandsHistoryStartAgain")}
          </Button>
        )}
        {history.hasMore ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            data-testid={`errands-history-load-more-${parentSessionId}`}
            disabled={history.status === "loading" || history.stale}
            onClick={onLoadMore}
            className={cn("min-h-8", drawer && ERRAND_TOUCH_TARGET)}
          >
            {t("task:errandsHistoryLoadMore")}
          </Button>
        ) : (
          <span
            data-testid={`errands-history-end-${parentSessionId}`}
            className="text-[11px] text-muted-foreground"
          >
            {t("task:errandsHistoryEnd")}
          </span>
        )}
      </div>
    </div>
  );
}
