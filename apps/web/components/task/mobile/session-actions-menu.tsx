"use client";

import { useRef, type RefObject } from "react";
import { IconDotsVertical } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  isSessionDeletable,
  isSessionResumable,
  isSessionStoppable,
} from "@/hooks/domains/session/use-session-actions";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TaskSessionState } from "@/lib/types/http";
import { HandoffDropdownMenuSub } from "../handoff-profile-menu-items";
import { useSessionMenuOpeningGesture } from "./use-session-menu-opening-gesture";

export type SessionActionsMenuProps = {
  triggerRef: RefObject<HTMLButtonElement | null>;
  taskId: string;
  state: TaskSessionState | null;
  isPrimary: boolean;
  /** An errand is never made primary and is not handed off to another profile. */
  isErrand?: boolean;
  onSetPrimary: () => void;
  onStop: () => void;
  onResume: () => void;
  onRestart: () => void;
  isRestarting: boolean;
  onAskDelete: () => void;
  onHandoffProfile: (profileId: string) => void;
};

function SessionLifecycleItems({
  state,
  isRestarting,
  onStop,
  onRestart,
  onResume,
  onDelete,
}: {
  state: TaskSessionState | null;
  isRestarting: boolean;
  onStop: () => void;
  onRestart: () => void;
  onResume: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {state && isSessionStoppable(state) && (
        <DropdownMenuItem className="cursor-pointer" onSelect={onStop} disabled={isRestarting}>
          {t("task:stop")}
        </DropdownMenuItem>
      )}
      {(isRestarting || (state && isSessionStoppable(state))) && (
        <DropdownMenuItem
          className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onSelect={onRestart}
          disabled={isRestarting}
        >
          {t("task:restartSession")}
        </DropdownMenuItem>
      )}
      {state && isSessionResumable(state) && (
        <DropdownMenuItem className="cursor-pointer" onSelect={onResume} disabled={isRestarting}>
          {t("task:resume")}
        </DropdownMenuItem>
      )}
      {state && isSessionDeletable(state) && (
        <DropdownMenuItem
          className="cursor-pointer text-destructive focus:text-destructive"
          disabled={isRestarting}
          onSelect={onDelete}
        >
          {t("task:delete")}
        </DropdownMenuItem>
      )}
    </>
  );
}

/** Exposes touch-accessible session actions and blocks conflicting lifecycle requests during restart. */
export function SessionActionsMenu({
  triggerRef,
  taskId,
  state,
  isPrimary,
  isErrand = false,
  onSetPrimary,
  onStop,
  onResume,
  onRestart,
  isRestarting,
  onAskDelete,
  onHandoffProfile,
}: SessionActionsMenuProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const pendingDelete = useRef(false);
  const { open, setOpen, triggerProps } = useSessionMenuOpeningGesture();
  const hasLifecycleAction =
    !!state &&
    (isSessionStoppable(state) || isSessionResumable(state) || isSessionDeletable(state));

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          ref={triggerRef}
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer h-11 w-11 md:h-7 md:w-7 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          {...triggerProps}
          aria-label={t("task:sessionActions")}
        >
          <IconDotsVertical className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onClick={(e) => e.stopPropagation()}
        onCloseAutoFocus={(event) => {
          if (!pendingDelete.current) return;
          event.preventDefault();
          pendingDelete.current = false;
          onAskDelete();
        }}
      >
        {!isErrand && (
          <>
            <DropdownMenuItem
              className="cursor-pointer"
              onSelect={onSetPrimary}
              disabled={isPrimary || !state}
            >
              {t("task:setAsPrimary")}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        <SessionLifecycleItems
          state={state}
          isRestarting={isRestarting}
          onStop={onStop}
          onRestart={onRestart}
          onResume={onResume}
          onDelete={() => {
            if (isMobile) pendingDelete.current = true;
            else onAskDelete();
          }}
        />
        {!isErrand && (
          <>
            {hasLifecycleAction && <DropdownMenuSeparator />}
            <HandoffDropdownMenuSub taskId={taskId} onSelectProfile={onHandoffProfile} />
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
