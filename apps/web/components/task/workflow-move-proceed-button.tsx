"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type MouseEvent as ReactMouseEvent,
  type MouseEventHandler as ReactMouseEventHandler,
  type RefObject,
} from "react";
import { IconArrowRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  useHoverIntentAffordance,
  HOVER_INTENT_CLOSE_DELAY_MS,
} from "./use-hover-intent-affordance";
import { WorkflowMoveAnchoredOptions, WorkflowMoveOptions } from "./workflow-move-options";
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
  nextStepName,
  isMoving,
  testId,
  submit,
  triggerRef,
  children,
}: {
  affordance: ReturnType<typeof useHoverIntentAffordance>;
  nextStepName: string;
  isMoving: boolean;
  testId: string;
  submit: (options?: WorkflowMoveEntryOptions) => Promise<boolean>;
  triggerRef: RefObject<HTMLButtonElement | null>;
  children: React.ReactNode;
}) {
  return (
    <>
      {children}
      <WorkflowMoveAnchoredOptions
        open={affordance.open}
        anchorRef={triggerRef}
        contentRef={affordance.contentRef}
        focusReturnRef={triggerRef}
        targetStepName={nextStepName}
        isMoving={isMoving}
        onOpenChange={(open) => {
          if (!open) affordance.requestClose(true);
        }}
        onSubmit={async (options) => {
          const ok = await submit(options);
          if (ok) affordance.requestClose();
          return ok;
        }}
        testId={`${testId}-options`}
        interactionProps={{
          onPointerEnter: affordance.contentProps.onPointerEnter,
          onPointerLeave: affordance.contentProps.onPointerLeave,
          onPointerDownCapture: affordance.contentProps.onPointerDownCapture,
          onFocusCapture: affordance.contentProps.onFocusCapture,
        }}
      />
    </>
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
      if (triggerRef.current?.isConnected && !triggerRef.current.disabled)
        triggerRef.current.focus();
    }, 0);
  }, []);

  return { triggerRef, scheduleDrawerFocusReturn };
}

type WorkflowMoveProceedButtonSurfaceProps = {
  nextStepName: string;
  isMoving: boolean;
  className?: string;
  testId: string;
  usesTouchDrawer: boolean;
  optionsOpen: boolean;
  affordance: ReturnType<typeof useHoverIntentAffordance>;
  triggerRef: RefObject<HTMLButtonElement | null>;
  onOptionsOpenChange: (open: boolean) => void;
  onDirectClick: (event: ReactMouseEvent<HTMLButtonElement>) => void;
  pointerHandlers: LongPressPointerHandlers;
  onTouchSurfaceClickCapture: ReactMouseEventHandler<HTMLDivElement>;
  submit: (options?: WorkflowMoveEntryOptions) => Promise<boolean>;
};

function WorkflowMoveProceedButtonSurface({
  nextStepName,
  isMoving,
  className,
  testId,
  usesTouchDrawer,
  optionsOpen,
  affordance,
  triggerRef,
  onOptionsOpenChange,
  onDirectClick,
  pointerHandlers,
  onTouchSurfaceClickCapture,
  submit,
}: WorkflowMoveProceedButtonSurfaceProps) {
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
      onClick={onDirectClick}
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

  if (usesTouchDrawer) {
    return (
      <ProceedTouchSurface
        button={button}
        open={optionsOpen}
        onOpenChange={onOptionsOpenChange}
        targetStepName={nextStepName}
        isMoving={isMoving}
        submit={submit}
        onReleaseClickCapture={onTouchSurfaceClickCapture}
      />
    );
  }

  return (
    <div className="inline-flex items-center gap-1">
      <ProceedDesktopSurface
        nextStepName={nextStepName}
        affordance={affordance}
        isMoving={isMoving}
        testId={testId}
        triggerRef={triggerRef}
        submit={submit}
      >
        {button}
      </ProceedDesktopSurface>
    </div>
  );
}

type WorkflowMoveProceedButtonProps = {
  nextStepName: string;
  onProceed: (options?: WorkflowMoveEntryOptions) => boolean | void | Promise<boolean | void>;
  isMoving: boolean;
  className?: string;
  testId: string;
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
}: WorkflowMoveProceedButtonProps) {
  const usesTouchDrawer = useTouchDrawer();
  const affordance = useHoverIntentAffordance();
  const [optionsOpen, setOptionsOpen] = useState(false);
  const proceedInFlightRef = useRef(false);
  const { triggerRef, scheduleDrawerFocusReturn } = useDrawerFocusReturn();
  const { pointerHandlers, consumePendingClick } = useWorkflowMoveLongPress(() => {
    setOptionsOpen(true);
  });

  useEffect(() => {
    if (!isMoving) proceedInFlightRef.current = false;
  }, [isMoving]);

  const handleDrawerOpenChange = useCallback(
    (open: boolean) => {
      setOptionsOpen(open);
      if (!open) {
        scheduleDrawerFocusReturn();
      }
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

  return (
    <WorkflowMoveProceedButtonSurface
      nextStepName={nextStepName}
      isMoving={isMoving}
      className={className}
      testId={testId}
      usesTouchDrawer={usesTouchDrawer}
      optionsOpen={optionsOpen}
      affordance={affordance}
      triggerRef={triggerRef}
      onOptionsOpenChange={handleDrawerOpenChange}
      onDirectClick={handleClick}
      pointerHandlers={pointerHandlers}
      onTouchSurfaceClickCapture={handleTouchSurfaceClickCapture}
      submit={tryProceed}
    />
  );
}
