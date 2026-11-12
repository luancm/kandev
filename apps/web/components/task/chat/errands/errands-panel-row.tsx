"use client";

import { useId } from "react";
import { IconChevronRight, IconLoader2, IconPlayerStop } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useToast } from "@/components/toast-provider";
import {
  deriveContextWindowUsage,
  formatContextTokens,
  type ContextWindowUsage,
} from "@/lib/context-window-usage";
import { useSessionContextWindow } from "@/hooks/domains/session/use-session-context-window";
import { useErrandActions } from "@/hooks/domains/session/use-errand-actions";
import { isLiveErrand, type Errand } from "@/lib/types/errands";
import { cn } from "@/lib/utils";
import { errandElapsedLabel } from "./errand-display";
import { ErrandProfileLabel } from "./errand-profile-label";
import { ERRAND_TOUCH_TARGET, ErrandOpenButton } from "./errand-open-button";
import { ErrandRecoveryNotice, ErrandStatusLabel, errandStatusLabelKey } from "./errand-status";
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
  deliveryUncertaintyDescriptionId: string;
  recoveryDescriptionId: string;
  contextSize: number;
  contextUsed: number;
};

function contextBarColor(percent: number): string {
  if (percent >= 90) return "bg-yellow-500";
  if (percent >= 75) return "bg-yellow-300";
  if (percent >= 50) return "bg-blue-500";
  return "bg-blue-300";
}

function RowSummary({
  errand,
  nowMs,
  title,
  contextUsage,
  contextDescriptionId,
  deliveryUncertaintyDescriptionId,
  recoveryDescriptionId,
  contextSize,
  contextUsed,
}: {
  errand: Errand;
  nowMs: number;
  title: string;
  contextUsage: ContextWindowUsage | undefined;
  contextDescriptionId: string;
  deliveryUncertaintyDescriptionId: string;
  recoveryDescriptionId: string;
  contextSize: number;
  contextUsed: number;
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
        <ErrandStatusLabel
          status={errand.status}
          attentionReason={errand.attention_reason}
          testId="errand-status"
        />
        {elapsed && (
          <span className="text-[11px] tabular-nums text-muted-foreground">{elapsed}</span>
        )}
      </span>
      {contextUsage && (
        <span
          id={contextDescriptionId}
          aria-label={
            contextUsage.unmeasured
              ? `${t("task:contextWindowNotMeasured")} ${t("task:contextWindowUsagePendingTokens", { size: contextSize.toLocaleString() })}`
              : `${t("task:contextWindowUsed", { percent: contextUsage.roundedPercent })} ${t("task:contextWindowUsedTokens", { used: contextUsed.toLocaleString(), size: contextSize.toLocaleString() })}`
          }
          className="max-w-full break-words text-[11px] leading-4 text-muted-foreground"
        >
          {contextUsage.unmeasured
            ? `${t("task:contextWindowNotMeasured")} (${t("task:contextWindowUsagePendingTokens", { size: formatContextTokens(contextSize) })})`
            : `${t("task:contextWindow")}: ${t("task:contextWindowUsedTokens", {
                used: formatContextTokens(contextUsed),
                size: formatContextTokens(contextSize),
              })}`}
          {!contextUsage.unmeasured && (
            <span
              aria-hidden="true"
              className="mt-1 block h-1 w-full overflow-hidden rounded-full bg-muted"
            >
              <span
                className={cn(
                  "block h-full rounded-full",
                  contextBarColor(contextUsage.usagePercent),
                )}
                style={{ width: `${contextUsage.usagePercent}%` }}
              />
            </span>
          )}
        </span>
      )}
      {errand.delivery_state === "uncertain" && (
        <span
          id={deliveryUncertaintyDescriptionId}
          data-testid="errand-delivery-uncertain"
          className="text-[11px] leading-4 text-amber-700 dark:text-amber-300"
        >
          {t("task:errandResultDeliveryUncertain")}
        </span>
      )}
      {errand.status === "needs_attention" && errand.attention_reason === "recovery_unresolved" && (
        <ErrandRecoveryNotice id={recoveryDescriptionId} />
      )}
    </span>
  );
}

const ROW_SURFACE = "rounded-md border border-border/60 bg-muted/20";

function ErrandStopButton({
  errandId,
  title,
  drawer,
}: {
  errandId: string;
  title: string;
  drawer: boolean;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { cancelErrand, isPending } = useErrandActions();
  const pending = isPending("cancel", errandId);

  return (
    <>
      <Button
        type="button"
        variant="destructive"
        size="sm"
        aria-label={t("task:errandStopAria", { title })}
        aria-busy={pending}
        disabled={pending}
        data-testid={`errand-stop-${errandId}`}
        onClick={() => {
          cancelErrand(errandId).catch(() => {
            toast({ title: t("task:errandStopFailed"), variant: "error" });
          });
        }}
        className={cn(
          "h-7 shrink-0 cursor-pointer gap-1 px-2 text-[11px]",
          drawer && ERRAND_TOUCH_TARGET,
        )}
      >
        {pending ? (
          <IconLoader2 aria-hidden className="h-3 w-3 animate-spin" />
        ) : (
          <IconPlayerStop aria-hidden className="h-3 w-3" />
        )}
        {t("task:errandStop")}
      </Button>
      <span role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        {pending ? t("task:errandStopProgress", { title }) : ""}
      </span>
    </>
  );
}

function DrawerRow({
  errand,
  taskId,
  nowMs,
  onOpened,
  contextUsage,
  contextDescriptionId,
  deliveryUncertaintyDescriptionId,
  recoveryDescriptionId,
  contextSize,
  contextUsed,
}: ErrandsPanelRowPresentationProps) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  const title = errand.title || t("task:errandUntitled");
  const status = t(
    errandStatusLabelKey(errand.status, { attentionReason: errand.attention_reason }),
  );
  const descriptions: string[] = [];
  if (contextUsage) descriptions.push(contextDescriptionId);
  if (errand.delivery_state === "uncertain") descriptions.push(deliveryUncertaintyDescriptionId);
  if (errand.attention_reason === "recovery_unresolved") descriptions.push(recoveryDescriptionId);
  return (
    <li>
      <div
        data-testid={`errands-panel-row-actions-${errand.errand_id}`}
        className={cn(ROW_SURFACE, "flex min-w-0 items-center gap-2 px-2 py-1.5")}
      >
        <button
          type="button"
          disabled={errand.status === "queued"}
          aria-label={t("chat:errandOpenWithStatusAria", { title, status })}
          aria-describedby={descriptions.join(" ") || undefined}
          data-testid={`errands-panel-row-${errand.errand_id}`}
          onClick={() => {
            openSession({ taskId, sessionId: errand.errand_id, title });
            onOpened();
          }}
          className={cn(
            "flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-3 rounded-md px-1.5 py-2 transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:cursor-default disabled:opacity-70",
          )}
        >
          <RowSummary
            errand={errand}
            nowMs={nowMs}
            title={title}
            contextUsage={contextUsage}
            contextDescriptionId={contextDescriptionId}
            deliveryUncertaintyDescriptionId={deliveryUncertaintyDescriptionId}
            recoveryDescriptionId={recoveryDescriptionId}
            contextSize={contextSize}
            contextUsed={contextUsed}
          />
          {errand.status !== "queued" && (
            <IconChevronRight aria-hidden className="h-4 w-4 shrink-0 text-muted-foreground" />
          )}
        </button>
        {isLiveErrand(errand) && (
          <ErrandStopButton errandId={errand.errand_id} title={title} drawer />
        )}
      </div>
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
  deliveryUncertaintyDescriptionId,
  recoveryDescriptionId,
  contextSize,
  contextUsed,
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
        deliveryUncertaintyDescriptionId={deliveryUncertaintyDescriptionId}
        recoveryDescriptionId={recoveryDescriptionId}
        contextSize={contextSize}
        contextUsed={contextUsed}
      />
      <div className="flex shrink-0 items-center gap-1">
        <ErrandOpenButton
          taskId={taskId}
          errandId={errand.errand_id}
          title={title}
          queued={errand.status === "queued"}
          onOpened={onOpened}
          describedBy={
            [
              contextUsage ? contextDescriptionId : null,
              errand.delivery_state === "uncertain" ? deliveryUncertaintyDescriptionId : null,
              errand.attention_reason === "recovery_unresolved" ? recoveryDescriptionId : null,
            ]
              .filter(Boolean)
              .join(" ") || undefined
          }
        />
        {isLiveErrand(errand) && (
          <ErrandStopButton errandId={errand.errand_id} title={title} drawer={false} />
        )}
      </div>
    </li>
  );
}

/** One errand in the chip panel: a card with **Open**, or a full-width row on phones. */
export function ErrandsPanelRow({ drawer, ...rest }: ErrandsPanelRowProps) {
  const contextWindow = useSessionContextWindow(rest.errand.errand_id, false);
  const contextUsage = deriveContextWindowUsage(contextWindow);
  const contextDescriptionId = useId();
  const deliveryUncertaintyDescriptionId = useId();
  const recoveryDescriptionId = useId();
  const presentationProps = {
    ...rest,
    contextUsage,
    contextDescriptionId,
    deliveryUncertaintyDescriptionId,
    recoveryDescriptionId,
    contextSize: contextWindow?.size ?? 0,
    contextUsed: contextWindow?.used ?? 0,
  };

  return drawer ? <DrawerRow {...presentationProps} /> : <PopoverRow {...presentationProps} />;
}
