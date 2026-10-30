"use client";

import { formatPromptDuration } from "@/lib/turn-duration";

import { useState } from "react";
import {
  IconChevronDown,
  IconChevronRight,
  IconHourglassHigh,
  IconPlayerStop,
  IconSubtask,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import { ERRAND_TOUCH_TARGET, ErrandOpenButton } from "../../errands/errand-open-button";
import {
  parseAwaitErrands,
  parseCancelErrand,
  parseStartErrand,
  type AwaitErrandRow,
} from "../../errands/errand-parse";
import { ErrandProfileLabel } from "../../errands/errand-profile-label";
import { ErrandTag } from "../../errands/errand-tag";
import {
  ErrandStatusLabel,
  errandStatusLabelKey,
  type ErrandStatusKind,
} from "../../errands/errand-status";
import { useLiveErrand, useSessionTaskId } from "../../errands/use-errand-lookup";
import { useOpenTaskSession } from "../../errands/use-open-task-session";
import { shortId } from "./parse";
import { IdChip, KandevBody, KandevRow, type KandevStatus } from "./shared";
import type { KandevRenderer, KandevRendererProps } from "./types";

const CARD_CLASS = "w-full min-w-0 rounded-md border border-border/60 bg-muted/20 text-xs";

function isInFlight(status: KandevStatus): boolean {
  return status === "pending" || status === "running";
}

function QuotedSubject({ subject }: { subject: string | null }) {
  if (!subject) return null;
  return <span className="max-w-[40ch] truncate">&ldquo;{subject}&rdquo;</span>;
}

/** Plain Kandev row for a failed or abandoned errand call; tool text renders as text only. */
function ErrandToolRow({
  titleKey,
  subject,
  status,
  errorText = null,
}: {
  titleKey: string;
  subject: string | null;
  status: KandevStatus;
  errorText?: string | null;
}) {
  const { t } = useTranslation();
  const hasError = errorText !== null;
  return (
    <div data-testid="errand-tool-row">
      <KandevRow
        Icon={IconSubtask}
        title={t(titleKey)}
        summary={<QuotedSubject subject={subject} />}
        status={hasError ? "error" : status}
        hasExpandableContent={Boolean(errorText)}
      >
        <KandevBody>
          <p className="whitespace-pre-wrap break-words text-xs text-destructive [overflow-wrap:anywhere]">
            {errorText}
          </p>
        </KandevBody>
      </KandevRow>
    </div>
  );
}

type StartCardModel = {
  taskId: string | null;
  errandId: string | null;
  title: string;
  objective: string | null;
  status: ErrandStatusKind;
  profileId: string | null;
  profileName: string | null;
};

function ObjectiveBody({ text }: { text: string }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 border-t border-border/40 px-3 py-2">
      <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
        {t("task:errandObjective")}
      </div>
      <p className="max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-xs text-foreground/80 [overflow-wrap:anywhere]">
        {text}
      </p>
    </div>
  );
}

function StartCardIdentity({
  model,
  expandable,
  expanded,
}: {
  model: StartCardModel;
  expandable: boolean;
  expanded: boolean;
}) {
  const Chevron = expanded ? IconChevronDown : IconChevronRight;
  return (
    <>
      {expandable && (
        <Chevron aria-hidden className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
      )}
      <ErrandTag kind="errand" />
      <span className="min-w-0 truncate font-medium text-foreground" title={model.title}>
        {model.title}
      </span>
      <ErrandProfileLabel
        profileId={model.profileId}
        fallbackName={model.profileName}
        className="min-w-0 max-w-[45%] shrink text-[11px]"
      />
    </>
  );
}

function StartErrandDesktopCard({ model }: { model: StartCardModel }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const expandable = Boolean(model.objective);
  const identity = <StartCardIdentity model={model} expandable={expandable} expanded={expanded} />;
  return (
    <div
      data-testid="errand-start-card"
      data-errand-id={model.errandId ?? undefined}
      className={CARD_CLASS}
    >
      <div className="flex min-w-0 items-center gap-2 px-1.5 py-1">
        {expandable ? (
          <button
            type="button"
            aria-expanded={expanded}
            onClick={() => setExpanded((value) => !value)}
            className={cn(
              "flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-left transition-colors hover:bg-muted/40",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
              ERRAND_TOUCH_TARGET,
            )}
          >
            {identity}
            <span className="sr-only">
              {t(expanded ? "chat:errandHideObjective" : "chat:errandShowObjective")}
            </span>
          </button>
        ) : (
          <div className="flex min-w-0 flex-1 items-center gap-2 px-1 py-0.5">{identity}</div>
        )}
        <ErrandStatusLabel status={model.status} testId="errand-start-status" />
        {model.errandId && (
          <ErrandOpenButton
            taskId={model.taskId}
            errandId={model.errandId}
            title={model.title}
            queued={model.status === "queued"}
          />
        )}
      </div>
      {expanded && model.objective && <ObjectiveBody text={model.objective} />}
    </div>
  );
}

/** Phones make the whole card the Open target, matching tappable session rows. */
function StartErrandPhoneCard({ model }: { model: StartCardModel }) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  const { taskId, errandId } = model;
  const canOpen = Boolean(taskId && errandId) && model.status !== "queued";
  return (
    <button
      type="button"
      data-testid="errand-start-card"
      data-errand-id={errandId ?? undefined}
      disabled={!canOpen}
      aria-label={t("chat:errandOpenWithStatusAria", {
        title: model.title,
        status: t(errandStatusLabelKey(model.status)),
      })}
      onClick={() => {
        if (taskId && errandId) openSession({ taskId, sessionId: errandId, title: model.title });
      }}
      className={cn(
        CARD_CLASS,
        "flex min-h-11 cursor-pointer items-center gap-2 px-2.5 py-2 text-left transition-colors active:bg-muted/50 disabled:cursor-default",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
      )}
    >
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="flex min-w-0 items-center gap-2">
          <ErrandTag kind="errand" />
          <span className="min-w-0 truncate font-medium text-foreground">{model.title}</span>
        </span>
        <span className="flex min-w-0 items-center gap-2">
          <ErrandStatusLabel status={model.status} testId="errand-start-status" />
          <ErrandProfileLabel
            profileId={model.profileId}
            fallbackName={model.profileName}
            showLogo={false}
            className="text-[11px]"
          />
        </span>
      </span>
      {canOpen && (
        <IconChevronRight aria-hidden className="h-4 w-4 shrink-0 text-muted-foreground" />
      )}
    </button>
  );
}

function StartErrandCard({ args, result, status, sessionId }: KandevRendererProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const view = parseStartErrand(args, result, status);
  const taskId = useSessionTaskId(sessionId);
  const live = useLiveErrand(taskId, view.errandId);
  if (view.errorText !== null || (!view.errandId && !isInFlight(status))) {
    return (
      <ErrandToolRow
        titleKey="task:kandevStartErrand"
        subject={view.title}
        status={status}
        errorText={view.errorText}
      />
    );
  }
  const model: StartCardModel = {
    taskId,
    errandId: view.errandId,
    title: live?.title || view.title || t("task:errandUntitled"),
    objective: live?.objective || view.prompt,
    status: live?.status ?? view.status ?? "starting",
    profileId: live?.agent_profile_id || view.profileId,
    profileName: view.profileName,
  };
  return isMobile ? (
    <StartErrandPhoneCard model={model} />
  ) : (
    <StartErrandDesktopCard model={model} />
  );
}

function AwaitErrandRowItem({
  row,
  taskId,
  awaitInFlight,
}: {
  row: AwaitErrandRow;
  taskId: string | null;
  awaitInFlight: boolean;
}) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  const live = useLiveErrand(taskId, row.errandId);
  const title = live?.title || shortId(row.errandId);
  const status: ErrandStatusKind | null =
    (awaitInFlight ? live?.status : row.status) ??
    row.status ??
    (row.pending || awaitInFlight ? "running" : null);
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
      {status && <ErrandStatusLabel status={status} />}
      {row.resultReceived && !awaitInFlight && (
        <span className="text-muted-foreground">{t("chat:errandAwaitResultReturned")}</span>
      )}
    </li>
  );
}

function awaitHeadingKey(inFlight: boolean, mode: "all" | "any"): string {
  if (!inFlight) return "chat:errandAwaitDone";
  return mode === "any" ? "chat:errandAwaitWaitingAny" : "chat:errandAwaitWaiting";
}

function AwaitErrandsCard({ args, result, status, sessionId }: KandevRendererProps) {
  const { t } = useTranslation();
  const view = parseAwaitErrands(args, result, status);
  const taskId = useSessionTaskId(sessionId);
  if (view.errorText !== null) {
    return (
      <ErrandToolRow
        titleKey="task:kandevAwaitErrands"
        subject={null}
        status={status}
        errorText={view.errorText}
      />
    );
  }
  const inFlight = isInFlight(status);
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
    <div data-testid="errand-await-card" className={CARD_CLASS}>
      <div className="flex min-w-0 flex-wrap items-center gap-2 px-2.5 py-1.5">
        <IconHourglassHigh aria-hidden className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 truncate font-medium text-foreground">
          {t(awaitHeadingKey(inFlight, view.mode), { count: view.rows.length })}
        </span>
        {!inFlight && duration !== null && (
          <span className="shrink-0 text-muted-foreground">{duration}</span>
        )}
        {!inFlight && view.endReason && (
          <span className="text-muted-foreground">{t(reasonKeys[view.endReason])}</span>
        )}
        {inFlight && (
          <span aria-hidden="true" className="inline-flex shrink-0">
            <GridSpinner className="text-[12px] text-muted-foreground" />
          </span>
        )}
      </div>
      {view.rows.length > 0 && (
        <ul className="divide-y divide-border/40 border-t border-border/40">
          {view.rows.map((row) => (
            <AwaitErrandRowItem
              key={row.errandId}
              row={row}
              taskId={taskId}
              awaitInFlight={inFlight}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function CancelErrandRow({ args, result, status, sessionId }: KandevRendererProps) {
  const { t } = useTranslation();
  const view = parseCancelErrand(args, result, status);
  const taskId = useSessionTaskId(sessionId);
  const live = useLiveErrand(taskId, view.errandId);
  if (view.errorText !== null) {
    return (
      <ErrandToolRow
        titleKey="task:kandevCancelErrand"
        subject={live?.title ?? null}
        status={status}
        errorText={view.errorText}
      />
    );
  }
  return (
    <div data-testid="errand-cancel-row">
      <KandevRow
        Icon={IconPlayerStop}
        title={t("task:kandevCancelErrand")}
        summary={
          live?.title ? (
            <QuotedSubject subject={live.title} />
          ) : (
            <IdChip id={view.errandId ?? undefined} />
          )
        }
        status={status}
        hasExpandableContent={false}
      />
    </div>
  );
}

// Each renderer returns a component element so its hooks run in their own
// component, never in the dispatcher that invokes renderers as functions.
export const StartErrandRenderer: KandevRenderer = (props) => <StartErrandCard {...props} />;
export const AwaitErrandsRenderer: KandevRenderer = (props) => <AwaitErrandsCard {...props} />;
export const CancelErrandRenderer: KandevRenderer = (props) => <CancelErrandRow {...props} />;
