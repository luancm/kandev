import { useLayoutEffect, useRef, type RefObject } from "react";
import { scheduleAfterPanelRestore } from "./transcript-auto-scroll";

type ReaderSnapshot = {
  sessionId: string;
  generation: number;
  scroller: HTMLElement;
  top: number;
  height: number;
  anchorId: string | null;
  anchorOffset: number | null;
};

function matchesReader(saved: ReaderSnapshot | null, sessionId: string | null, generation: number) {
  return saved?.sessionId === sessionId && saved?.generation === generation;
}

function captureReader(panel: HTMLElement | null, sessionId: string, generation: number) {
  const scroller = panel?.querySelector<HTMLElement>(".chat-message-list");
  if (!scroller || scroller.clientHeight === 0) return null;
  if (scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 3) return null;
  const bounds = scroller.getBoundingClientRect();
  const anchor = Array.from(scroller.querySelectorAll<HTMLElement>('[id^="msg-"]')).find((row) => {
    const rect = row.getBoundingClientRect();
    return rect.bottom > bounds.top && rect.top < bounds.bottom;
  });
  return {
    sessionId,
    generation,
    scroller,
    top: scroller.scrollTop,
    height: scroller.scrollHeight,
    anchorId: anchor?.id ?? null,
    anchorOffset: anchor ? anchor.getBoundingClientRect().top - bounds.top : null,
  };
}

function restoreReader(saved: ReaderSnapshot) {
  const { scroller, anchorId, anchorOffset } = saved;
  const anchor = anchorId
    ? Array.from(scroller.querySelectorAll<HTMLElement>('[id^="msg-"]')).find(
        (row) => row.id === anchorId,
      )
    : null;
  if (anchor && anchorOffset !== null) {
    const offset = anchor.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
    const delta = offset - anchorOffset;
    if (Math.abs(delta) > 0.5) scroller.scrollTop += delta;
    return true;
  } else if (scroller.scrollHeight === saved.height) {
    if (Math.abs(scroller.scrollTop - saved.top) > 0.5) scroller.scrollTop = saved.top;
    return true;
  }
  return false;
}

function ownReader(
  saved: ReaderSnapshot,
  panel: HTMLElement,
  snapshot: { current: ReaderSnapshot | null },
) {
  let cancelPending: (() => void) | null = null;
  let released = false;
  const previousAnchor = saved.scroller.style.overflowAnchor;
  saved.scroller.style.overflowAnchor = "none";
  const events = ["wheel", "touchstart", "pointerdown", "keydown"] as const;
  const observer = new ResizeObserver(scheduleRestore);
  function release() {
    released = true;
    cancelPending?.();
    observer.disconnect();
    saved.scroller.removeEventListener("scroll", scheduleRestore);
    for (const event of events) panel.removeEventListener(event, interrupt, true);
    saved.scroller.style.overflowAnchor = previousAnchor;
  }
  function interrupt() {
    snapshot.current = null;
    release();
  }
  function scheduleRestore() {
    if (released || cancelPending) return;
    cancelPending = scheduleAfterPanelRestore(() => {
      cancelPending = null;
      if (snapshot.current !== saved || !panel.contains(saved.scroller)) return interrupt();
      if (!restoreReader(saved)) interrupt();
    });
  }
  observer.observe(saved.scroller);
  const content = saved.scroller.querySelector("[data-chat-content]");
  if (content) observer.observe(content);
  saved.scroller.addEventListener("scroll", scheduleRestore, { passive: true });
  for (const event of events)
    panel.addEventListener(event, interrupt, { passive: true, capture: true });
  scheduleRestore();
  return release;
}

/** The returning parent owns its reading anchor until newer input or navigation takes over. */
export function useRetainedReaderPosition({
  panelRef,
  sessionId,
  generation,
  inactive,
  hasTarget,
  hasLocalTarget = false,
  visible = true,
}: {
  panelRef: RefObject<HTMLDivElement | null>;
  sessionId: string | null;
  generation: number;
  inactive: boolean;
  hasTarget: boolean;
  hasLocalTarget?: boolean;
  visible?: boolean;
}) {
  const previousInactive = useRef(inactive);
  const snapshot = useRef<ReaderSnapshot | null>(null);
  const positionOwned = hasTarget || hasLocalTarget;
  useLayoutEffect(() => {
    const wasInactive = previousInactive.current;
    previousInactive.current = inactive;
    if (!inactive && !visible) {
      snapshot.current = null;
      return;
    }
    if (positionOwned || !matchesReader(snapshot.current, sessionId, generation))
      snapshot.current = null;
    if (inactive) {
      if (!wasInactive && !snapshot.current && sessionId && !positionOwned)
        snapshot.current = captureReader(panelRef.current, sessionId, generation);
      return;
    }
    const saved = snapshot.current;
    if (!wasInactive || !saved || !panelRef.current) return;
    return ownReader(saved, panelRef.current, snapshot);
  }, [generation, positionOwned, inactive, panelRef, sessionId, visible]);
}
