"use client";

import { IconPlayerStop, IconSubtask } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { ErrandOpenButton } from "../../errands/errand-open-button";
import { parseAwaitErrands, parseCancelErrand, parseStartErrand } from "../../errands/errand-parse";
import { ErrandHistoryDisclosure } from "../../errands/errand-history-disclosure";
import { AwaitErrandsDisplay, ErrandSnapshotStatus } from "../../errands/errand-await";
import { ErrandProfileLabel } from "../../errands/errand-profile-label";
import { ErrandStatusLabel, type ErrandStatusKind } from "../../errands/errand-status";
import { useLiveErrand, useSessionTaskId } from "../../errands/use-errand-lookup";
import { IdChip, KandevBody, KandevRow, type KandevStatus } from "./shared";
import type { KandevRenderer, KandevRendererProps } from "./types";

const CARD_CLASS = "w-full min-w-0 rounded-md border border-border/60 bg-muted/20 text-xs";

function isInFlight(status: KandevStatus): boolean {
  return status === "pending" || status === "running";
}

function QuotedSubject({ subject }: { subject: string | null }) {
  if (!subject) return null;
  return <q className="inline-block max-w-[40ch] truncate">{subject}</q>;
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
  canOpen: boolean;
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

function StartErrandHistory({ model }: { model: StartCardModel }) {
  const { t } = useTranslation();
  return (
    <ErrandHistoryDisclosure
      testId="errand-start-card"
      errandId={model.errandId}
      summary={t("chat:errandStarted", { count: 1 })}
    >
      <div className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1 break-words text-foreground/80 [overflow-wrap:anywhere]">
          {model.title}
        </span>
        {model.errandId && (
          <ErrandOpenButton
            taskId={model.taskId}
            errandId={model.errandId}
            title={model.title}
            queued={!model.canOpen}
          />
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <ErrandProfileLabel
          profileId={model.profileId}
          fallbackName={model.profileName}
          className="text-[11px]"
        />
        {model.status !== "starting" && (
          <ErrandSnapshotStatus status={model.status} testId="errand-start-status" />
        )}
      </div>
      {model.objective && <ObjectiveBody text={model.objective} />}
    </ErrandHistoryDisclosure>
  );
}

function StartErrandCard({ args, result, status, sessionId }: KandevRendererProps) {
  const { t } = useTranslation();
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
    title: view.title || t("task:errandUntitled"),
    objective: view.prompt,
    status: view.status ?? "starting",
    profileId: view.profileId,
    profileName: view.profileName,
    canOpen: (live?.status ?? view.status) !== "queued",
  };
  if (isInFlight(status)) {
    return (
      <div data-testid="errand-start-card" className={CARD_CLASS}>
        <ErrandStatusLabel status="starting" testId="errand-start-status" />
      </div>
    );
  }
  return <StartErrandHistory model={model} />;
}

function AwaitErrandsCard({ args, result, status, sessionId }: KandevRendererProps) {
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
  return <AwaitErrandsDisplay view={view} taskId={taskId} inFlight={isInFlight(status)} />;
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
