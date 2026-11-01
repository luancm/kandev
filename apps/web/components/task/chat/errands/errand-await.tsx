"use client";

import { IconChevronRight, IconClockHour4, IconHourglassHigh } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";
import { formatPromptDuration } from "@/lib/turn-duration";
import { cn } from "@/lib/utils";
import { shortId } from "../messages/kandev/parse";
import { ERRAND_TOUCH_TARGET, ErrandOpenButton } from "./errand-open-button";
import {
  awaitErrandStatus,
  type AwaitErrandRow,
  type AwaitErrandStatus,
  type AwaitErrandsView,
} from "./errand-parse";
import { ErrandStatusIcon, ErrandStatusLabel } from "./errand-status";
import { useLiveErrand } from "./use-errand-lookup";
import { useOpenTaskSession } from "./use-open-task-session";

export function ErrandSnapshotStatus({
  status,
  testId,
  cancelledByUser = false,
}: {
  status: AwaitErrandStatus;
  testId?: string;
  cancelledByUser?: boolean;
}) {
  const { t } = useTranslation();
  const labelKeys = {
    pending: "chat:errandAwaitNotFinished",
    running: "chat:errandAwaitWasRunning",
    queued: "chat:errandAwaitWasQueued",
    needs_attention: "chat:errandAwaitNeededAttention",
    done: "task:errandStatusDone",
    failed: "task:errandStatusFailed",
    cancelled: "task:errandStatusCancelled",
  } as const;
  return (
    <span
      data-testid={testId}
      data-errand-status={status}
      className="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
    >
      {status === "pending" || status === "running" ? (
        <IconClockHour4 aria-hidden className="h-3.5 w-3.5 shrink-0" />
      ) : (
        <ErrandStatusIcon status={status} />
      )}
      {t(
        status === "cancelled" && cancelledByUser
          ? "task:errandStatusCancelledByYou"
          : labelKeys[status],
      )}
    </span>
  );
}

function AwaitErrandRowItem({
  row,
  taskId,
  inFlight,
}: {
  row: AwaitErrandRow;
  taskId: string | null;
  inFlight: boolean;
}) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  const live = useLiveErrand(taskId, row.errandId);
  const title = live?.title || shortId(row.errandId);
  const status = awaitErrandStatus(row, live?.status, inFlight);
  if (!inFlight) {
    return (
      <li data-testid={`errand-await-row-${row.errandId}`} className="min-w-0 px-2.5 py-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <span className="min-w-0 flex-1 break-words text-foreground/80 [overflow-wrap:anywhere]">
            {title}
          </span>
          <ErrandOpenButton
            taskId={taskId}
            errandId={row.errandId}
            title={title}
            queued={live ? live.status === "queued" : status === "queued"}
          />
        </div>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          {status && <ErrandSnapshotStatus status={status} />}
          {row.resultReceived && (
            <span className="text-muted-foreground">{t("chat:errandAwaitResultReturned")}</span>
          )}
        </div>
        {row.resultText && (
          <p
            data-testid="errand-await-result"
            className="mt-1 whitespace-pre-wrap break-words text-foreground/80 [overflow-wrap:anywhere]"
          >
            {row.resultText}
          </p>
        )}
      </li>
    );
  }
  return (
    <li
      data-testid={`errand-await-row-${row.errandId}`}
      className="flex min-w-0 items-center gap-2 px-2.5 py-1"
    >
      <button
        type="button"
        disabled={!taskId || status === "queued"}
        aria-label={t("chat:errandOpenAria", { title })}
        onClick={() => {
          if (taskId) openSession({ taskId, sessionId: row.errandId, title });
        }}
        className={cn(
          "min-w-0 flex-1 cursor-pointer truncate rounded text-left text-foreground hover:underline disabled:cursor-default disabled:no-underline",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          ERRAND_TOUCH_TARGET,
        )}
      >
        {title}
      </button>
      {status && status !== "pending" && <ErrandStatusLabel status={status} />}
    </li>
  );
}

function AwaitErrandList({
  view,
  taskId,
  inFlight,
}: {
  view: AwaitErrandsView;
  taskId: string | null;
  inFlight: boolean;
}) {
  if (view.rows.length === 0) return null;
  return (
    <ul className="divide-y divide-border/40">
      {view.rows.map((row) => (
        <AwaitErrandRowItem key={row.errandId} row={row} taskId={taskId} inFlight={inFlight} />
      ))}
    </ul>
  );
}

function AwaitHistory({ view, taskId }: { view: AwaitErrandsView; taskId: string | null }) {
  const { t } = useTranslation();
  const reasonKeys = {
    all_ready: "chat:errandAwaitAllReady",
    any_ready: "chat:errandAwaitAnyReady",
    timeout: "chat:errandAwaitTimeout",
    parent_ended: "chat:errandAwaitParentEnded",
  } as const;
  const duration =
    view.durationMs === null
      ? null
      : formatPromptDuration(view.durationMs / 1000, {
          s: t("task:durationUnitSeconds"),
          m: t("task:durationUnitMinutes"),
          h: t("task:durationUnitHours"),
        });
  return (
    <Collapsible data-testid="errand-await-card" className="w-full min-w-0 text-xs">
      <CollapsibleTrigger
        data-testid="errand-await-disclosure"
        className={cn(
          "group flex w-full min-w-0 cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-muted-foreground hover:bg-muted/30",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          ERRAND_TOUCH_TARGET,
        )}
      >
        <IconChevronRight
          aria-hidden
          className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60 group-data-[state=open]:rotate-90"
        />
        <IconClockHour4 aria-hidden className="h-3.5 w-3.5 shrink-0" />
        <span className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-1 [overflow-wrap:anywhere]">
          <span>
            {t(duration === null ? "chat:errandAwaitDone" : "chat:errandAwaitElapsed", {
              count: view.rows.length,
              duration,
            })}
          </span>
          {view.endReason && (
            <>
              <span aria-hidden>·</span>
              <span>{t(reasonKeys[view.endReason])}</span>
            </>
          )}
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mx-2 border-t border-border/40 pt-2">
          <p className="px-2.5 text-[11px] text-muted-foreground">
            {t("chat:errandAwaitSnapshot")}
          </p>
          <AwaitErrandList view={view} taskId={taskId} inFlight={false} />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function AwaitErrandsDisplay({
  view,
  taskId,
  inFlight,
}: {
  view: AwaitErrandsView;
  taskId: string | null;
  inFlight: boolean;
}) {
  const { t } = useTranslation();
  if (!inFlight) return <AwaitHistory view={view} taskId={taskId} />;
  return (
    <div
      data-testid="errand-await-card"
      className="w-full min-w-0 rounded-md border border-border/60 bg-muted/20 text-xs"
    >
      <div className="flex min-w-0 flex-wrap items-center gap-2 px-2.5 py-1.5">
        <IconHourglassHigh aria-hidden className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 truncate font-medium text-foreground">
          {t(view.mode === "any" ? "chat:errandAwaitWaitingAny" : "chat:errandAwaitWaiting", {
            count: view.rows.length,
          })}
        </span>
        <span aria-hidden="true" className="inline-flex shrink-0">
          <GridSpinner className="text-[12px] text-muted-foreground" />
        </span>
      </div>
      {view.rows.length > 0 && (
        <div className="border-t border-border/40">
          <AwaitErrandList view={view} taskId={taskId} inFlight />
        </div>
      )}
    </div>
  );
}
