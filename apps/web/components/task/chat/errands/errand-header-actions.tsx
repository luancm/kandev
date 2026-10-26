"use client";

import { useCallback } from "react";
import {
  IconCornerUpLeft,
  IconDotsVertical,
  IconLoader2,
  IconPlayerStop,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useToast } from "@/components/toast-provider";
import { useErrandActions } from "@/hooks/domains/session/use-errand-actions";
import { canSendErrandResult, isLiveErrand, type Errand } from "@/lib/types/errands";
import { cn } from "@/lib/utils";
import { useSessionMenuOpeningGesture } from "../../mobile/use-session-menu-opening-gesture";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";

export type ErrandHeaderAction = "stop" | "send_result" | null;

/** **Stop** while live, **Send result to parent** when eligible, never both. */
export function errandHeaderAction(errand: Errand | null): ErrandHeaderAction {
  if (!errand) return null;
  if (isLiveErrand(errand)) return "stop";
  return canSendErrandResult(errand) ? "send_result" : null;
}

function useErrandHeaderActions(sessionId: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { cancelErrand, sendErrandResult, isPending } = useErrandActions();
  const stop = useCallback(() => {
    cancelErrand(sessionId).catch(() => {
      toast({ title: t("task:errandStopFailed"), variant: "error" });
    });
  }, [cancelErrand, sessionId, t, toast]);
  const sendResult = useCallback(() => {
    sendErrandResult(sessionId)
      .then(() => toast({ title: t("task:errandResultSent"), variant: "success" }))
      .catch(() => toast({ title: t("task:errandSendResultFailed"), variant: "error" }));
  }, [sendErrandResult, sessionId, t, toast]);
  return {
    stop,
    sendResult,
    stopPending: isPending("cancel", sessionId),
    sendPending: isPending("send_result", sessionId),
  };
}

function PendingIcon({ pending, Icon }: { pending: boolean; Icon: typeof IconPlayerStop }) {
  if (pending) return <IconLoader2 aria-hidden className="h-3 w-3 animate-spin" />;
  return <Icon aria-hidden className="h-3 w-3" />;
}

/** Desktop header action button. */
export function ErrandHeaderActionButton({
  errand,
  sessionId,
}: {
  errand: Errand | null;
  sessionId: string;
}) {
  const { t } = useTranslation();
  const actions = useErrandHeaderActions(sessionId);
  const action = errandHeaderAction(errand);
  if (action === "stop") {
    return (
      <Button
        type="button"
        variant="destructive"
        size="sm"
        disabled={actions.stopPending}
        onClick={actions.stop}
        data-testid="errand-stop-button"
        className={cn("h-6 shrink-0 cursor-pointer gap-1 px-2 text-[11px]", ERRAND_TOUCH_TARGET)}
      >
        <PendingIcon pending={actions.stopPending} Icon={IconPlayerStop} />
        {t("task:errandStop")}
      </Button>
    );
  }
  if (action === "send_result") {
    return (
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={actions.sendPending}
        onClick={actions.sendResult}
        data-testid="errand-send-result-button"
        className={cn("h-6 shrink-0 cursor-pointer gap-1 px-2 text-[11px]", ERRAND_TOUCH_TARGET)}
      >
        <PendingIcon pending={actions.sendPending} Icon={IconCornerUpLeft} />
        {t("task:errandSendResult")}
      </Button>
    );
  }
  return null;
}

/** Phone header actions: a visible menu holding the one applicable action. */
export function ErrandHeaderActionsMenu({
  errand,
  sessionId,
}: {
  errand: Errand | null;
  sessionId: string;
}) {
  const { t } = useTranslation();
  const actions = useErrandHeaderActions(sessionId);
  const { open, setOpen, triggerProps } = useSessionMenuOpeningGesture();
  const action = errandHeaderAction(errand);
  if (!action) return <span aria-hidden="true" className="h-11 w-11 shrink-0" />;
  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("task:errandActions")}
          data-testid="errand-actions-menu"
          className="h-11 w-11 shrink-0 cursor-pointer"
          {...triggerProps}
        >
          <IconDotsVertical aria-hidden className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {action === "stop" ? (
          <DropdownMenuItem
            className="min-h-11 cursor-pointer text-destructive focus:text-destructive"
            disabled={actions.stopPending}
            onSelect={actions.stop}
          >
            <IconPlayerStop aria-hidden className="h-4 w-4" />
            {t("task:errandStop")}
          </DropdownMenuItem>
        ) : (
          <DropdownMenuItem
            className="min-h-11 cursor-pointer"
            disabled={actions.sendPending}
            onSelect={actions.sendResult}
          >
            <IconCornerUpLeft aria-hidden className="h-4 w-4" />
            {t("task:errandSendResult")}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
