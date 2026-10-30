"use client";

import { useId } from "react";
import { IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { deriveContextWindowUsage, type ContextWindowUsage } from "@/lib/context-window-usage";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import type { Errand } from "@/lib/types/errands";
import { cn } from "@/lib/utils";
import { errandElapsedLabel } from "./errand-display";
import { ErrandProfileLabel } from "./errand-profile-label";
import { ErrandOpenButton } from "./errand-open-button";
import { ErrandStatusLabel, errandStatusLabelKey } from "./errand-status";
import { useOpenTaskSession } from "./use-open-task-session";

type ErrandsPanelRowProps = {
  errand: Errand;
  taskId: string;
  /** Epoch ms shared by every row so a live panel ticks from one timer. */
  nowMs: number;
  /** Phone and coarse-pointer form: the whole row is one 44px target. */
  drawer: boolean;
  /** Called after the errand was opened, to close the surface. */
  onOpened: () => void;
};

type ErrandsPanelRowPresentationProps = Omit<ErrandsPanelRowProps, "drawer"> & {
  contextUsage: ContextWindowUsage | undefined;
  contextDescriptionId: string;
};

function RowSummary({
  errand,
  nowMs,
  title,
  contextUsage,
  contextDescriptionId,
}: {
  errand: Errand;
  nowMs: number;
  title: string;
  contextUsage: ContextWindowUsage | undefined;
  contextDescriptionId: string;
}) {
  const { t } = useTranslation();
  const elapsed = errandElapsedLabel(errand, nowMs);
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5 text-left">
      <span className="truncate text-xs font-medium text-foreground" title={title}>
        {title}
      </span>
      <ErrandProfileLabel
        profileId={errand.agent_profile_id}
        showLogo={false}
        className="max-w-full text-[11px]"
      />
      <span className="flex items-center gap-1.5">
        <ErrandStatusLabel status={errand.status} />
        {elapsed && (
          <span className="text-[11px] tabular-nums text-muted-foreground">{elapsed}</span>
        )}
      </span>
      {contextUsage && (
        <span
          id={contextDescriptionId}
          className="max-w-full break-words text-[11px] leading-4 text-muted-foreground"
        >
          {contextUsage.unmeasured
            ? t("task:contextWindowNotMeasured")
            : t("task:contextWindowUsed", { percent: contextUsage.roundedPercent })}
        </span>
      )}
    </span>
  );
}

const ROW_SURFACE = "rounded-md border border-border/60 bg-muted/20";

function DrawerRow({
  errand,
  taskId,
  nowMs,
  onOpened,
  contextUsage,
  contextDescriptionId,
}: ErrandsPanelRowPresentationProps) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  const title = errand.title || t("task:errandUntitled");
  const status = t(errandStatusLabelKey(errand.status));
  return (
    <li>
      <button
        type="button"
        disabled={errand.status === "queued"}
        aria-label={t("chat:errandOpenWithStatusAria", { title, status })}
        aria-describedby={contextUsage ? contextDescriptionId : undefined}
        data-testid={`errands-panel-row-${errand.errand_id}`}
        onClick={() => {
          openSession({ taskId, sessionId: errand.errand_id, title });
          onOpened();
        }}
        className={cn(
          ROW_SURFACE,
          "flex min-h-11 w-full cursor-pointer items-center gap-3 px-3 py-2 transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-default disabled:opacity-70",
        )}
      >
        <RowSummary
          errand={errand}
          nowMs={nowMs}
          title={title}
          contextUsage={contextUsage}
          contextDescriptionId={contextDescriptionId}
        />
        {errand.status !== "queued" && (
          <IconChevronRight aria-hidden className="h-4 w-4 shrink-0 text-muted-foreground" />
        )}
      </button>
    </li>
  );
}

function PopoverRow({
  errand,
  taskId,
  nowMs,
  onOpened,
  contextUsage,
  contextDescriptionId,
}: ErrandsPanelRowPresentationProps) {
  const { t } = useTranslation();
  const title = errand.title || t("task:errandUntitled");
  return (
    <li
      data-testid={`errands-panel-row-${errand.errand_id}`}
      className={cn(ROW_SURFACE, "flex items-center justify-between gap-2 px-2 py-1.5")}
    >
      <RowSummary
        errand={errand}
        nowMs={nowMs}
        title={title}
        contextUsage={contextUsage}
        contextDescriptionId={contextDescriptionId}
      />
      <ErrandOpenButton
        taskId={taskId}
        errandId={errand.errand_id}
        title={title}
        queued={errand.status === "queued"}
        onOpened={onOpened}
      />
    </li>
  );
}

/** One errand in the chip panel: a card with **Open**, or a full-width row on phones. */
export function ErrandsPanelRow({ drawer, ...rest }: ErrandsPanelRowProps) {
  const contextWindow = useSessionContextWindow(rest.errand.errand_id, false);
  const contextUsage = deriveContextWindowUsage(contextWindow);
  const contextDescriptionId = useId();
  const presentationProps = { ...rest, contextUsage, contextDescriptionId };

  return drawer ? <DrawerRow {...presentationProps} /> : <PopoverRow {...presentationProps} />;
}
