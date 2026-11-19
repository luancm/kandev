"use client";

import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCircleCheck,
  IconCircleX,
  IconClockHour4,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import type { Errand, ErrandStatus } from "@/lib/types/errands";
import { cn } from "@/lib/utils";

/** An errand status, or `starting` while its start call has not returned yet. */
export type ErrandStatusKind = ErrandStatus | "starting";

type StatusPresentation = {
  labelKey: string;
  Icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }> | null;
  iconClassName: string;
  labelClassName: string;
};

const MUTED = "text-muted-foreground";

// Icons and hues follow the session-state vocabulary in `lib/ui/state-icons`:
// the grid spinner marks live agent work (session tabs, subagent cards), the
// amber shield marks a pending permission, green check marks completion, and
// the red triangle marks failure.
const PRESENTATION: Record<ErrandStatusKind, StatusPresentation> = {
  starting: {
    labelKey: "task:errandStatusStarting",
    Icon: null,
    iconClassName: MUTED,
    labelClassName: MUTED,
  },
  queued: {
    labelKey: "task:errandStatusQueued",
    Icon: IconClockHour4,
    iconClassName: MUTED,
    labelClassName: MUTED,
  },
  running: {
    labelKey: "task:errandStatusRunning",
    Icon: null,
    iconClassName: MUTED,
    labelClassName: "text-foreground/80",
  },
  needs_attention: {
    labelKey: "task:errandStatusNeedsAttention",
    Icon: IconShieldQuestion,
    iconClassName: "text-amber-500",
    labelClassName: "font-medium text-amber-700 dark:text-amber-300",
  },
  done: {
    labelKey: "task:errandStatusDone",
    Icon: IconCircleCheck,
    iconClassName: "text-green-500",
    labelClassName: MUTED,
  },
  failed: {
    labelKey: "task:errandStatusFailed",
    Icon: IconAlertTriangle,
    iconClassName: "text-red-500",
    labelClassName: MUTED,
  },
  cancelled: {
    labelKey: "task:errandStatusCancelled",
    Icon: IconCircleX,
    iconClassName: MUTED,
    labelClassName: MUTED,
  },
};

const CANCELLED_BY_YOU_KEY = "task:errandStatusCancelledByYou";

/** Catalog key of a status label. A delivered cancellation is always a user cancel. */
export function errandStatusLabelKey(
  status: ErrandStatusKind,
  options: { cancelledByUser?: boolean; attentionReason?: Errand["attention_reason"] } = {},
): string {
  if (status === "cancelled" && options.cancelledByUser) return CANCELLED_BY_YOU_KEY;
  if (status === "needs_attention" && options.attentionReason === "recovery_unresolved") {
    return "task:errandStatusRecoveryUnresolved";
  }
  return PRESENTATION[status].labelKey;
}

/** Decorative status glyph; the adjacent label carries the accessible text. */
export function ErrandStatusIcon({
  status,
  attentionReason,
  className,
}: {
  status: ErrandStatusKind;
  attentionReason?: Errand["attention_reason"];
  className?: string;
}) {
  const { Icon, iconClassName } =
    status === "needs_attention" && attentionReason === "recovery_unresolved"
      ? {
          ...PRESENTATION.needs_attention,
          Icon: IconAlertTriangle,
          iconClassName: "text-amber-500",
        }
      : PRESENTATION[status];
  if (!Icon) {
    return (
      <span aria-hidden="true" className={cn("inline-flex shrink-0", className)}>
        <GridSpinner className={cn("text-[12px]", iconClassName)} />
      </span>
    );
  }
  return <Icon aria-hidden className={cn("h-3.5 w-3.5 shrink-0", iconClassName, className)} />;
}

/** Icon plus localized label, styled like the session state badges. */
export function ErrandStatusLabel({
  status,
  attentionReason,
  cancelledByUser = false,
  className,
  testId,
}: {
  status: ErrandStatusKind;
  attentionReason?: Errand["attention_reason"];
  cancelledByUser?: boolean;
  className?: string;
  testId?: string;
}) {
  const { t } = useTranslation();
  return (
    <span
      data-testid={testId}
      data-errand-status={status}
      data-errand-attention-reason={attentionReason}
      className={cn(
        "inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-[11px] leading-none",
        PRESENTATION[status].labelClassName,
        className,
      )}
    >
      <ErrandStatusIcon status={status} attentionReason={attentionReason} />
      {t(errandStatusLabelKey(status, { cancelledByUser, attentionReason }))}
    </span>
  );
}

/** Short recovery explanation shared by the Errands list and open child header. */
export function ErrandRecoveryNotice({ id, className }: { id?: string; className?: string }) {
  const { t } = useTranslation();
  return (
    <p
      id={id}
      role="note"
      data-testid="errand-recovery-notice"
      className={cn(
        "flex min-w-0 items-start gap-1 text-[11px] leading-4 text-amber-800 dark:text-amber-200",
        className,
      )}
    >
      <IconAlertTriangle aria-hidden className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
      <span>{t("task:errandRecoveryExplanation")}</span>
    </p>
  );
}
