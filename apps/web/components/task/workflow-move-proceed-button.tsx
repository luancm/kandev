"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type MouseEvent as ReactMouseEvent,
} from "react";
import { IconArrowRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  useHoverIntentAffordance,
  HOVER_INTENT_CLOSE_DELAY_MS,
} from "./use-hover-intent-affordance";
import { WorkflowMoveOptions, WorkflowMoveOptionsForm } from "./workflow-move-options";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

export const WORKFLOW_MOVE_LONG_PRESS_MS = 450;
export const WORKFLOW_MOVE_LONG_PRESS_SLOP_PX = 10;
export { HOVER_INTENT_CLOSE_DELAY_MS as WORKFLOW_MOVE_AFFORDANCE_CLOSE_DELAY_MS };

export function isWithinLongPressSlop(
  startX: number,
  startY: number,
  x: number,
  y: number,
): boolean {
  const dx = x - startX;
  const dy = y - startY;
  return Math.sqrt(dx * dx + dy * dy) <= WORKFLOW_MOVE_LONG_PRESS_SLOP_PX;
}

type LongPressPointerHandlers = {
  onPointerDown: (event: ReactPointerEvent<HTMLElement>) => void;
  onPointerMove: (event: ReactPointerEvent<HTMLElement>) => void;
  onPointerUp: (event: ReactPointerEvent<HTMLElement>) => void;
  onPointerCancel: (event: ReactPointerEvent<HTMLElement>) => void;
};

/**
 * Long-press gesture for the coarse-pointer proceed button. A short tap must
 * still fire the normal click, so the hook only reports "this click belongs
 * to a completed long press" through `consumePendingClick`, which the caller
 * checks (and consumes) in its click handler.
 */
export function useWorkflowMoveLongPress(onLongPress: () => void): {
  pointerHandlers: LongPressPointerHandlers;
  consumePendingClick: () => boolean;
} {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const originRef = useRef<{ x: number; y: number } | null>(null);
  const pendingClickRef = useRef(false);
  const onLongPressRef = useRef(onLongPress);
  onLongPressRef.current = onLongPress;

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    originRef.current = null;
  }, []);

  // Unmount cancels any in-flight long press and its pending click.
  useEffect(
    () => () => {
      clearTimer();
      pendingClickRef.current = false;
    },
    [clearTimer],
  );

  const onPointerDown = useCallback(
    (event: ReactPointerEvent<HTMLElement>) => {
      pendingClickRef.current = false;
      // Mouse pointers use the desktop hover affordance instead; secondary
      // buttons never start a long press.
      if (event.pointerType === "mouse" || event.button !== 0) {
        clearTimer();
        return;
      }
      clearTimer();
      originRef.current = { x: event.clientX, y: event.clientY };
      timerRef.current = setTimeout(() => {
        originRef.current = null;
        pendingClickRef.current = true;
        onLongPressRef.current();
      }, WORKFLOW_MOVE_LONG_PRESS_MS);
    },
    [clearTimer],
  );

  const onPointerMove = useCallback(
    (event: ReactPointerEvent<HTMLElement>) => {
      const origin = originRef.current;
      if (!origin) return;
      if (!isWithinLongPressSlop(origin.x, origin.y, event.clientX, event.clientY)) {
        clearTimer();
      }
    },
    [clearTimer],
  );

  const onPointerUp = useCallback(() => clearTimer(), [clearTimer]);
  const onPointerCancel = useCallback(() => clearTimer(), [clearTimer]);

  const consumePendingClick = useCallback(() => {
    if (!pendingClickRef.current) return false;
    pendingClickRef.current = false;
    return true;
  }, []);

  return {
    pointerHandlers: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel },
    consumePendingClick,
  };
}

type ProceedDesktopSurfaceProps = {
  affordance: ReturnType<typeof useHoverIntentAffordance>;
  isMoving: boolean;
  optionsTestId: string;
  hint: string;
  submit: (options?: WorkflowMoveEntryOptions) => Promise<boolean>;
  children: React.ReactNode;
};

/** Coarse-pointer surface: long press opens the shared options Drawer. */
function ProceedTouchSurface({
  button,
  open,
  onOpenChange,
  targetStepName,
  isMoving,
  submit,
  onReleaseClickCapture,
}: {
  button: React.ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targetStepName: string;
  isMoving: boolean;
  submit: (options?: WorkflowMoveEntryOptions) => Promise<boolean>;
  onReleaseClickCapture: React.MouseEventHandler<HTMLDivElement>;
}) {
  return (
    <div className="contents" onClickCapture={onReleaseClickCapture}>
      {button}
      <WorkflowMoveOptions
        open={open}
        onOpenChange={onOpenChange}
        targetStepName={targetStepName}
        isMoving={isMoving}
        onSubmit={async (options) => {
          const ok = await submit(options);
          if (ok) onOpenChange(false);
        }}
      />
    </div>
  );
}

/** Fine-pointer surface: hover/focus reveals the options form in a popover. */
function ProceedDesktopSurface({
  affordance,
  isMoving,
  optionsTestId,
  hint,
  submit,
  children,
}: ProceedDesktopSurfaceProps) {
  return (
    // All dismiss paths (escape, outside pointer, focus) are intercepted in
    // popoverContentProps; an onOpenChange mirror would re-run requestClose
    // without focus-restore and clobber the keyboard-close semantics.
    <Popover open={affordance.open}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        className="w-80 max-w-[calc(100vw-2rem)] gap-0 p-3"
        align="end"
        {...affordance.popoverContentProps}
      >
        <div
          {...affordance.contentProps}
          className="flex flex-col gap-2.5"
          data-testid={optionsTestId}
        >
          <p className="text-xs text-muted-foreground">{hint}</p>
          <WorkflowMoveOptionsForm
            isMoving={isMoving}
            isTouchSurface={false}
            instructionsRows={3}
            onSubmit={async (options) => {
              const ok = await submit(options);
              if (ok) affordance.requestClose();
              return ok;
            }}
          />
        </div>
      </PopoverContent>
    </Popover>
  );
}

/** Returns the trigger ref plus a deferred focus restore for the triggerless Drawer close. */
function useDrawerFocusReturn() {
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const focusRestoreTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (focusRestoreTimerRef.current !== null) clearTimeout(focusRestoreTimerRef.current);
    },
    [],
  );

  const scheduleDrawerFocusReturn = useCallback(() => {
    if (focusRestoreTimerRef.current !== null) {
      clearTimeout(focusRestoreTimerRef.current);
    }
    focusRestoreTimerRef.current = setTimeout(() => {
      focusRestoreTimerRef.current = null;
      const trigger = triggerRef.current;
      if (trigger?.isConnected && !trigger.disabled) trigger.focus();
    }, 0);
  }, []);

  return { triggerRef, scheduleDrawerFocusReturn };
}

type WorkflowMoveProceedButtonProps = {
  nextStepName: string;
  onProceed: (options?: WorkflowMoveEntryOptions) => boolean | void | Promise<boolean | void>;
  isMoving: boolean;
  className?: string;
  testId: string;
  optionsTestId?: string;
};

/**
 * One primary "move to next step" button. A plain click/tap moves immediately.
 * On fine-pointer desktop, hovering (or keyboard-focusing) the button reveals
 * the move-options form directly in a popover anchored to it. On coarse-pointer
 * devices, a long press opens the options drawer while a short tap keeps the
 * direct move. A failed move keeps the form and its draft open.
 */
export function WorkflowMoveProceedButton({
  nextStepName,
  onProceed,
  isMoving,
  className,
  testId,
  optionsTestId = `${testId}-options`,
}: WorkflowMoveProceedButtonProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const affordance = useHoverIntentAffordance();
  const [optionsOpen, setOptionsOpen] = useState(false);
  const proceedInFlightRef = useRef(false);
  const { triggerRef, scheduleDrawerFocusReturn } = useDrawerFocusReturn();
  const { pointerHandlers, consumePendingClick } = useWorkflowMoveLongPress(() =>
    setOptionsOpen(true),
  );

  useEffect(() => {
    if (!isMoving) proceedInFlightRef.current = false;
  }, [isMoving]);

  const handleDrawerOpenChange = useCallback(
    (open: boolean) => {
      setOptionsOpen(open);
      if (!open) scheduleDrawerFocusReturn();
    },
    [scheduleDrawerFocusReturn],
  );

  const tryProceed = useCallback(
    async (options?: WorkflowMoveEntryOptions): Promise<boolean> => {
      if (isMoving || proceedInFlightRef.current) return false;
      proceedInFlightRef.current = true;
      try {
        const result = await onProceed(options);
        return result !== false;
      } finally {
        proceedInFlightRef.current = false;
      }
    },
    [isMoving, onProceed],
  );

  const handleClick = (event: ReactMouseEvent<HTMLButtonElement>) => {
    if (consumePendingClick()) {
      // A completed long press produced this synthetic click; the options
      // surface is already open, so swallow the direct move.
      event.preventDefault();
      return;
    }
    // PopoverTrigger's default click toggles the controlled popover. The
    // button's primary action is the direct move, so prevent that toggle.
    event.preventDefault();
    affordance.requestClose();
    void tryProceed();
  };

  const handleTouchSurfaceClickCapture = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (!consumePendingClick()) return;
    // When a long press opens the Drawer under the held pointer, the browser's
    // compatibility click can retarget to the Drawer submit button on release.
    // Consume that one click before it can submit an empty options form.
    event.preventDefault();
    event.stopPropagation();
  };

  const button = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      ref={triggerRef}
      className={cn(
        "gap-1 px-2.5 text-xs cursor-pointer text-primary",
        usesTouchDrawer && "min-h-11",
        className,
      )}
      onClick={handleClick}
      disabled={isMoving}
      data-testid={testId}
      {...(usesTouchDrawer
        ? pointerHandlers
        : {
            onPointerEnter: affordance.triggerProps.onPointerEnter,
            onPointerLeave: affordance.triggerProps.onPointerLeave,
            onFocus: affordance.triggerProps.onFocus,
          })}
    >
      {nextStepName}
      <IconArrowRight className="h-3.5 w-3.5" />
    </Button>
  );

  // Coarse pointer: the options surface is the shared Drawer.
  if (usesTouchDrawer) {
    return (
      <ProceedTouchSurface
        button={button}
        open={optionsOpen}
        onOpenChange={handleDrawerOpenChange}
        targetStepName={nextStepName}
        isMoving={isMoving}
        submit={tryProceed}
        onReleaseClickCapture={handleTouchSurfaceClickCapture}
      />
    );
  }

  // Fine pointer: hover/focus reveals the options form directly in a popover.
  return (
    <ProceedDesktopSurface
      affordance={affordance}
      isMoving={isMoving}
      optionsTestId={optionsTestId}
      hint={t("task:moveTaskToTheNextWorkflow")}
      submit={tryProceed}
    >
      {button}
    </ProceedDesktopSurface>
  );
}
