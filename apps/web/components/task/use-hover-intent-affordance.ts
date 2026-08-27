"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";

export const HOVER_INTENT_OPEN_DELAY_MS = 200;
export const HOVER_INTENT_CLOSE_DELAY_MS = 100;

type HoverIntentAffordanceOptions = {
  /** Hover grace period before the surface opens; 0 opens immediately. */
  openDelayMs?: number;
  /** Grace period before closing after the pointer leaves trigger and content. */
  closeDelayMs?: number;
};

/** Tracks whether the last input was keyboard or pointer. */
function useKeyboardModality() {
  const keyboardModalityRef = useRef(false);
  useEffect(() => {
    const markKeyboardModality = () => {
      keyboardModalityRef.current = true;
    };
    const markPointerModality = () => {
      keyboardModalityRef.current = false;
    };
    document.addEventListener("keydown", markKeyboardModality, true);
    document.addEventListener("pointerdown", markPointerModality, true);
    return () => {
      document.removeEventListener("keydown", markKeyboardModality, true);
      document.removeEventListener("pointerdown", markPointerModality, true);
    };
  }, []);
  return keyboardModalityRef;
}

function buildTriggerProps({
  contentRef,
  keyboardModalityRef,
  suppressNextFocusOpenRef,
  openFromPointer,
  openFromKeyboard,
  scheduleClose,
}: {
  contentRef: React.RefObject<HTMLDivElement | null>;
  keyboardModalityRef: React.RefObject<boolean>;
  suppressNextFocusOpenRef: React.RefObject<boolean>;
  openFromPointer: () => void;
  openFromKeyboard: () => void;
  scheduleClose: () => void;
}) {
  return {
    onPointerEnter: () => openFromPointer(),
    onPointerLeave: (event: ReactPointerEvent<HTMLElement>) => {
      if (!contentRef.current?.contains(event.relatedTarget as Node | null)) scheduleClose();
    },
    onFocus: () => {
      // A focus restore back onto the trigger must not read as the user
      // focusing it to open the surface again.
      if (suppressNextFocusOpenRef.current) {
        suppressNextFocusOpenRef.current = false;
        return;
      }
      if (keyboardModalityRef.current) openFromKeyboard();
    },
  };
}

function buildContentProps({
  contentRef,
  keyboardOpenedRef,
  clearTimers,
  scheduleClose,
}: {
  contentRef: React.RefObject<HTMLDivElement | null>;
  keyboardOpenedRef: React.RefObject<boolean>;
  clearTimers: () => void;
  scheduleClose: () => void;
}) {
  return {
    ref: contentRef,
    onPointerEnter: () => clearTimers(),
    onPointerLeave: (event: ReactPointerEvent<HTMLElement>) => {
      if (keyboardOpenedRef.current) return;
      if (contentRef.current?.contains(event.relatedTarget as Node | null)) return;
      scheduleClose();
    },
  };
}

function buildPopoverContentProps({
  keyboardOpenedRef,
  requestClose,
}: {
  keyboardOpenedRef: React.RefObject<boolean>;
  requestClose: (restoreFocus?: boolean) => void;
}) {
  return {
    onOpenAutoFocus: (event: Event) => {
      // Keyboard focus jumps into the content so the form is reachable;
      // hover opening must not steal focus from the page.
      if (!keyboardOpenedRef.current) event.preventDefault();
    },
    onCloseAutoFocus: (event: Event) => {
      const restoreFocus = keyboardOpenedRef.current;
      keyboardOpenedRef.current = false;
      if (!restoreFocus) event.preventDefault();
    },
    onEscapeKeyDown: () => requestClose(true),
    onPointerDownOutside: () => requestClose(),
    onFocusOutside: () => requestClose(),
  };
}

/**
 * Hover-intent popover controller shared by the workflow move surfaces.
 *
 * Fine-pointer hover (after `openDelayMs`) or keyboard focus opens the
 * popover; pointer focus never does. Closing is delayed by `closeDelayMs`
 * so the pointer can cross the gap between trigger and content, and is
 * suppressed entirely while `pinned` — set when the user interacts with
 * content (or explicitly by the host) so an expanded form cannot be
 * dismissed by stray pointer movement. Escape keeps focus for keyboard
 * sessions; outside interactions do not.
 */
export function useHoverIntentAffordance({
  openDelayMs = 0,
  closeDelayMs = HOVER_INTENT_CLOSE_DELAY_MS,
}: HoverIntentAffordanceOptions = {}) {
  const [open, setOpen] = useState(false);
  const [pinned, setPinned] = useState(false);
  const keyboardModalityRef = useKeyboardModality();
  const keyboardOpenedRef = useRef(false);
  const pinnedRef = useRef(false);
  const suppressNextFocusOpenRef = useRef(false);
  const openTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const contentRef = useRef<HTMLDivElement | null>(null);
  pinnedRef.current = pinned;

  const clearTimers = useCallback(() => {
    if (openTimerRef.current !== null) {
      clearTimeout(openTimerRef.current);
      openTimerRef.current = null;
    }
    if (closeTimerRef.current !== null) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  }, []);

  useEffect(() => clearTimers, [clearTimers]);

  // A closed surface cannot be "in use": drop a stale pin so the next open
  // starts with normal hover-close behavior.
  useEffect(() => {
    if (!open) setPinned(false);
  }, [open]);

  const requestClose = useCallback(
    (restoreFocus = false) => {
      clearTimers();
      if (!restoreFocus) keyboardOpenedRef.current = false;
      else suppressNextFocusOpenRef.current = true;
      setOpen(false);
    },
    [clearTimers],
  );

  const scheduleClose = useCallback(() => {
    if (pinnedRef.current) return;
    clearTimers();
    closeTimerRef.current = setTimeout(() => {
      closeTimerRef.current = null;
      requestClose();
    }, closeDelayMs);
  }, [clearTimers, closeDelayMs, requestClose]);

  const openFromPointer = useCallback(() => {
    clearTimers();
    keyboardOpenedRef.current = false;
    suppressNextFocusOpenRef.current = false;
    if (openDelayMs <= 0) {
      setOpen(true);
      return;
    }
    openTimerRef.current = setTimeout(() => {
      openTimerRef.current = null;
      setOpen(true);
    }, openDelayMs);
  }, [clearTimers, openDelayMs]);

  const openFromKeyboard = useCallback(() => {
    clearTimers();
    keyboardOpenedRef.current = true;
    suppressNextFocusOpenRef.current = false;
    setOpen(true);
  }, [clearTimers]);

  const markContentInteraction = useCallback(() => setPinned(true), []);

  const triggerProps = buildTriggerProps({
    contentRef,
    keyboardModalityRef,
    suppressNextFocusOpenRef,
    openFromPointer,
    openFromKeyboard,
    scheduleClose,
  });

  const contentProps = {
    ...buildContentProps({ contentRef, keyboardOpenedRef, clearTimers, scheduleClose }),
    // Interacting with the content (fields, buttons) pins the surface for the
    // rest of the session so pointer travel cannot dismiss it mid-entry.
    onPointerDownCapture: markContentInteraction,
    onFocusCapture: markContentInteraction,
  };

  const popoverContentProps = buildPopoverContentProps({ keyboardOpenedRef, requestClose });

  return {
    open,
    pinned,
    setPinned,
    contentRef,
    triggerProps,
    contentProps,
    popoverContentProps,
    requestClose,
  };
}
