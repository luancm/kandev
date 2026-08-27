"use client";

import { useRef, type ReactNode, type RefObject } from "react";

import { Button } from "@kandev/ui/button";

import { AnchoredActionPopover } from "./anchored-action-popover";

export type ActionConfirmPopoverSize = "default" | "wide";

export type ActionConfirmPopoverProps = {
  open: boolean;
  size?: ActionConfirmPopoverSize;
  disabled?: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  title: ReactNode;
  description?: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  confirmDisabled?: boolean;
  testId?: string;
  confirmationBoundary?: boolean;
  onOpenChange: (open: boolean) => void;
  onCancel?: () => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Compatibility wrapper for destructive confirmations. The anchored shell
 * owns positioning, focus, and outside interaction; this component keeps the
 * historical close-before-callback confirmation semantics.
 */
export function ActionConfirmPopover({
  open,
  size = "default",
  disabled = false,
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  title,
  description,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  confirmDisabled = false,
  testId = "action-confirm-popover",
  confirmationBoundary = false,
  onOpenChange,
  onCancel,
  onConfirm,
}: ActionConfirmPopoverProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmedRef = useRef(false);
  const confirmIsDisabled = disabled || confirmDisabled;

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      confirmedRef.current = false;
      onOpenChange(true);
      return;
    }
    closeActionConfirm(confirmedRef, onCancel, onOpenChange);
  };

  const handleConfirm = () => {
    if (confirmIsDisabled) return;
    if (!isConnected(anchorRef.current)) {
      handleOpenChange(false);
      return;
    }
    confirmedRef.current = true;
    handleOpenChange(false);
    queueMicrotask(() => {
      void Promise.resolve()
        .then(onConfirm)
        .catch(() => undefined);
    });
  };

  return (
    <AnchoredActionPopover
      open={open}
      anchorRef={anchorRef}
      widthClassName={size === "wide" ? "w-72" : "w-64"}
      focusReturnRef={focusReturnRef}
      focusBoundaryRef={focusBoundaryRef}
      title={title}
      description={description}
      body={null}
      footer={
        <div className="flex justify-end gap-2">
          <Button
            ref={cancelRef}
            type="button"
            variant="outline"
            disabled={disabled}
            className="min-h-11 px-3 transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]"
            onClick={() => handleOpenChange(false)}
          >
            {cancelLabel}
          </Button>
          <Button
            type="button"
            variant="destructive"
            aria-label={confirmAriaLabel}
            data-testid={confirmTestId}
            disabled={confirmIsDisabled}
            className="min-h-11 px-3 transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]"
            onClick={handleConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      }
      initialFocusRef={cancelRef}
      confirmationBoundary={confirmationBoundary}
      returnFocus={() => !confirmedRef.current}
      testId={testId}
      onOpenChange={handleOpenChange}
      onDismiss={() => handleOpenChange(false)}
    />
  );
}

function closeActionConfirm(
  confirmedRef: { current: boolean },
  onCancel: (() => void) | undefined,
  onOpenChange: (open: boolean) => void,
) {
  if (!confirmedRef.current) onCancel?.();
  onOpenChange(false);
}

function isConnected(element: HTMLElement | null): element is HTMLElement {
  return element !== null && element.isConnected;
}
