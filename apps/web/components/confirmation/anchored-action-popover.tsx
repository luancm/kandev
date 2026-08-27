"use client";

import {
  createContext,
  useCallback,
  useId,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useContext,
  type ComponentProps,
  type HTMLAttributes,
  type ReactNode,
  type RefObject,
} from "react";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
} from "@kandev/ui/popover";
import { cn } from "@kandev/ui/lib/utils";

/** Default surface width; callers that shrink with their content override it. */
export const ANCHORED_ACTION_POPOVER_WIDTH = "w-[min(24rem,calc(100vw-1rem))]";

export type AnchoredActionPopoverProps = {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  /** Optional owner ref for hover/focus controllers that track this surface. */
  contentRef?: RefObject<HTMLDivElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  title: ReactNode;
  description?: ReactNode;
  body: ReactNode;
  footer?: ReactNode;
  initialFocusRef?: RefObject<HTMLElement | null>;
  additionalBoundaryRefs?: Array<RefObject<HTMLElement | null>>;
  interactionProps?: Pick<
    HTMLAttributes<HTMLDivElement>,
    "onPointerEnter" | "onPointerLeave" | "onPointerDownCapture" | "onFocusCapture"
  >;
  confirmationBoundary?: boolean;
  returnFocus?: boolean | (() => boolean);
  /** Width utility for the surface; defaults to the fixed form width. */
  widthClassName?: string;
  /** Remove the form shell chrome for compact action surfaces. */
  compact?: boolean;
  testId?: string;
  collisionPadding?: number;
  onOpenChange: (open: boolean) => void;
  onDismiss?: () => void;
};

const AnchoredActionPopoverContext = createContext<HTMLElement | null>(null);

/** Returns the outer content node for nested selector portals. */
export function useAnchoredActionPopoverPortalContainer(): HTMLElement | null {
  return useContext(AnchoredActionPopoverContext);
}

function createAnchoredActionHandlers({
  anchorRef,
  isInsideBoundary,
  dismiss,
  focusFirstControl,
  restoreFocus,
  hasPendingInternalInteraction,
  resetDismissed,
}: {
  anchorRef: RefObject<HTMLElement | null>;
  isInsideBoundary: (target: EventTarget | null) => boolean;
  dismiss: () => void;
  focusFirstControl: () => void;
  restoreFocus: () => void;
  hasPendingInternalInteraction: () => boolean;
  resetDismissed: () => void;
}): Pick<
  AnchoredActionPopoverContentProps,
  | "onOpenAutoFocus"
  | "onFocusOutside"
  | "onInteractOutside"
  | "onEscapeKeyDown"
  | "onKeyDownCapture"
  | "onCloseAutoFocus"
> {
  return {
    onOpenAutoFocus: (event) => {
      event.preventDefault();
      queueMicrotask(focusFirstControl);
    },
    onFocusOutside: (event) => {
      if (isInsideBoundary(event.target) || hasPendingInternalInteraction()) {
        event.preventDefault();
      }
    },
    onInteractOutside: (event) => {
      const target = event.target;
      if (
        anchorRef.current?.contains(target as Node) ||
        isInsideBoundary(target) ||
        hasPendingInternalInteraction()
      ) {
        event.preventDefault();
      }
    },
    onEscapeKeyDown: (event) => {
      event.preventDefault();
      dismiss();
    },
    onKeyDownCapture: (event) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      dismiss();
    },
    onCloseAutoFocus: (event) => {
      event.preventDefault();
      restoreFocus();
      resetDismissed();
    },
  };
}

function createAnchoredOpenChangeHandler({
  dismiss,
  resetDismissed,
  onOpenChange,
}: {
  dismiss: () => void;
  resetDismissed: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  return (nextOpen: boolean) => {
    if (nextOpen) {
      resetDismissed();
      onOpenChange(true);
      return;
    }
    dismiss();
  };
}

type AnchoredActionPopoverLifecycleOptions = Pick<
  AnchoredActionPopoverProps,
  | "open"
  | "anchorRef"
  | "contentRef"
  | "focusReturnRef"
  | "focusBoundaryRef"
  | "initialFocusRef"
  | "additionalBoundaryRefs"
  | "returnFocus"
  | "onOpenChange"
  | "onDismiss"
>;

// eslint-disable-next-line max-lines-per-function -- the lifecycle hook keeps dismissal, focus, and nested-boundary ownership together.
function useAnchoredActionPopoverLifecycle({
  open,
  anchorRef,
  contentRef: externalContentRef,
  focusReturnRef,
  focusBoundaryRef,
  initialFocusRef,
  additionalBoundaryRefs = [],
  returnFocus = true,
  onOpenChange,
  onDismiss,
}: AnchoredActionPopoverLifecycleOptions) {
  const titleId = useId();
  const descriptionId = useId();
  const internalContentRef = useRef<HTMLDivElement>(null);
  const contentRef = externalContentRef ?? internalContentRef;
  const [portalContainer, setPortalContainer] = useState<HTMLElement | null>(null);
  const dismissedRef = useRef(false);
  const dismissRef = useRef<() => void>(() => undefined);
  const internalInteractionRef = useRef(false);
  const isInsideBoundary = (target: EventTarget | null) => {
    if (!(target instanceof Node)) return false;
    return Boolean(
      contentRef.current?.contains(target) ||
      focusBoundaryRef?.current?.contains(target) ||
      additionalBoundaryRefs.some((boundaryRef) => boundaryRef.current?.contains(target)),
    );
  };
  const dismiss = () => {
    internalInteractionRef.current = false;
    if (!dismissedRef.current) {
      dismissedRef.current = true;
      onDismiss?.();
    }
    onOpenChange(false);
  };
  const markInternalInteraction = (target: EventTarget | null) => {
    if (!(target instanceof Node) || !contentRef.current?.contains(target)) return;
    internalInteractionRef.current = true;
    setTimeout(() => {
      internalInteractionRef.current = false;
    }, 0);
  };
  const focusFirstControl = () => {
    if (isConnected(initialFocusRef?.current ?? null)) {
      initialFocusRef?.current?.focus();
      return;
    }
    const firstControl = contentRef.current?.querySelector<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]):not([aria-disabled="true"])',
    );
    firstControl?.focus();
  };
  const restoreFocus = () => {
    const shouldReturn = typeof returnFocus === "function" ? returnFocus() : returnFocus;
    if (!shouldReturn) return;
    const focusTarget = focusReturnRef?.current ?? anchorRef.current;
    if (isConnected(focusTarget)) focusTarget.focus();
  };
  dismissRef.current = dismiss;

  useLayoutEffect(() => {
    if (!open) return;
    dismissedRef.current = false;
    if (!isConnected(anchorRef.current)) dismiss();
  });

  useEffect(() => {
    if (!open) return;
    const handleDocumentKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (!(event.target instanceof Node) || !contentRef.current?.contains(event.target)) return;
      event.preventDefault();
      event.stopPropagation();
      dismissRef.current();
    };
    document.addEventListener("keydown", handleDocumentKeyDown, true);
    return () => document.removeEventListener("keydown", handleDocumentKeyDown, true);
  }, [open]);

  const contentHandlers = createAnchoredActionHandlers({
    anchorRef,
    isInsideBoundary,
    dismiss,
    focusFirstControl,
    restoreFocus,
    hasPendingInternalInteraction: () => internalInteractionRef.current,
    resetDismissed: () => {
      dismissedRef.current = false;
    },
  });
  const handleOpenChange = createAnchoredOpenChangeHandler({
    dismiss,
    resetDismissed: () => {
      dismissedRef.current = false;
    },
    onOpenChange,
  });

  const setContentElement = useCallback(
    (element: HTMLDivElement | null) => {
      (contentRef as { current: HTMLDivElement | null }).current = element;
      setPortalContainer(element);
    },
    [contentRef],
  );

  return {
    titleId,
    descriptionId,
    contentRef,
    contentHandlers,
    handleOpenChange,
    portalContainer,
    markInternalInteraction,
    setContentElement,
  };
}

export function AnchoredActionPopover({
  open,
  anchorRef,
  contentRef: ownerContentRef,
  focusReturnRef,
  focusBoundaryRef,
  title,
  description,
  body,
  footer,
  initialFocusRef,
  additionalBoundaryRefs = [],
  interactionProps,
  confirmationBoundary = false,
  returnFocus = true,
  widthClassName = ANCHORED_ACTION_POPOVER_WIDTH,
  compact = false,
  testId = "anchored-action-popover",
  collisionPadding = 8,
  onOpenChange,
  onDismiss,
}: AnchoredActionPopoverProps) {
  const {
    titleId,
    descriptionId,
    contentHandlers,
    handleOpenChange,
    portalContainer,
    markInternalInteraction,
    setContentElement,
  } = useAnchoredActionPopoverLifecycle({
    open,
    anchorRef,
    contentRef: ownerContentRef,
    focusReturnRef,
    focusBoundaryRef,
    initialFocusRef,
    additionalBoundaryRefs,
    returnFocus,
    onOpenChange,
    onDismiss,
  });

  return (
    <Popover modal={false} open={open} onOpenChange={handleOpenChange}>
      <PopoverAnchor virtualRef={anchorRef as RefObject<HTMLElement>} />
      <AnchoredActionPopoverContext.Provider value={portalContainer}>
        <AnchoredActionPopoverContent
          contentElementRef={setContentElement}
          onInternalInteraction={markInternalInteraction}
          titleId={titleId}
          descriptionId={descriptionId}
          title={title}
          description={description}
          body={body}
          footer={footer}
          testId={testId}
          widthClassName={widthClassName}
          compact={compact}
          confirmationBoundary={confirmationBoundary}
          collisionPadding={collisionPadding}
          interactionProps={interactionProps}
          {...contentHandlers}
        />
      </AnchoredActionPopoverContext.Provider>
    </Popover>
  );
}

type AnchoredActionPopoverContentProps = {
  contentElementRef: (element: HTMLDivElement | null) => void;
  onInternalInteraction: (target: EventTarget | null) => void;
  titleId: string;
  descriptionId: string;
  title: ReactNode;
  description?: ReactNode;
  body: ReactNode;
  footer?: ReactNode;
  testId: string;
  widthClassName: string;
  compact: boolean;
  confirmationBoundary: boolean;
  collisionPadding: number;
  interactionProps?: AnchoredActionPopoverProps["interactionProps"];
} & Pick<
  ComponentProps<typeof PopoverContent>,
  | "onOpenAutoFocus"
  | "onFocusOutside"
  | "onInteractOutside"
  | "onEscapeKeyDown"
  | "onKeyDownCapture"
  | "onCloseAutoFocus"
>;

function AnchoredActionPopoverContent({
  contentElementRef,
  onInternalInteraction,
  titleId,
  descriptionId,
  title,
  description,
  body,
  footer,
  testId,
  widthClassName,
  compact,
  confirmationBoundary,
  collisionPadding,
  interactionProps,
  onOpenAutoFocus,
  onFocusOutside,
  onInteractOutside,
  onEscapeKeyDown,
  onKeyDownCapture,
  onCloseAutoFocus,
}: AnchoredActionPopoverContentProps) {
  return (
    <PopoverContent
      ref={contentElementRef}
      role="dialog"
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      data-testid={testId}
      data-confirmation-boundary={confirmationBoundary ? "" : undefined}
      side="bottom"
      align="end"
      sideOffset={8}
      collisionPadding={collisionPadding}
      className={cn(
        "flex min-w-0 max-w-[calc(100vw-1rem)] max-h-[var(--radix-popover-content-available-height)] flex-col gap-0 overflow-hidden",
        compact ? "p-1.5" : "p-0",
        widthClassName,
      )}
      onOpenAutoFocus={onOpenAutoFocus}
      onFocusOutside={onFocusOutside}
      onInteractOutside={onInteractOutside}
      onEscapeKeyDown={onEscapeKeyDown}
      onKeyDownCapture={onKeyDownCapture}
      onPointerEnter={interactionProps?.onPointerEnter}
      onPointerLeave={interactionProps?.onPointerLeave}
      onPointerDownCapture={(event) => {
        onInternalInteraction(event.target);
        interactionProps?.onPointerDownCapture?.(event);
      }}
      onFocusCapture={(event) => {
        onInternalInteraction(event.target);
        interactionProps?.onFocusCapture?.(event);
      }}
      onCloseAutoFocus={onCloseAutoFocus}
    >
      {compact ? (
        <>
          <PopoverTitle id={titleId} className="sr-only">
            {title}
          </PopoverTitle>
          {description ? (
            <PopoverDescription id={descriptionId} className="sr-only">
              {description}
            </PopoverDescription>
          ) : null}
        </>
      ) : (
        <PopoverHeader className="min-w-0 shrink-0 gap-1 border-b px-4 py-3">
          <PopoverTitle id={titleId} className="min-w-0 break-words">
            {title}
          </PopoverTitle>
          {description ? (
            <PopoverDescription id={descriptionId} className="min-w-0 break-words">
              {description}
            </PopoverDescription>
          ) : null}
        </PopoverHeader>
      )}
      <div
        data-slot="anchored-action-body"
        className={cn("min-h-0 min-w-0 flex-1 overflow-y-auto", compact ? "p-0" : "p-4")}
      >
        {body}
      </div>
      {footer ? (
        <div
          data-slot="anchored-action-footer"
          className="min-w-0 shrink-0 border-t bg-popover px-4 py-3"
        >
          {footer}
        </div>
      ) : null}
    </PopoverContent>
  );
}

function isConnected(element: HTMLElement | null): element is HTMLElement {
  return element !== null && element.isConnected;
}
