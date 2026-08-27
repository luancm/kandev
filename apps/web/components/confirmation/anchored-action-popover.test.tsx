import { createRef, useRef, useState, type ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AnchoredActionPopover } from "./anchored-action-popover";

function Harness({
  align,
  onDismiss = vi.fn(),
}: {
  align?: "start" | "center" | "end";
  onDismiss?: () => void;
}) {
  const [open, setOpen] = useState(true);
  const [showAnchor, setShowAnchor] = useState(true);
  const anchorRef = useRef<HTMLButtonElement>(null);
  const body: ReactNode = <input aria-label="First field" />;
  const footer = (
    <button type="button" onClick={() => setOpen(false)}>
      Cancel
    </button>
  );

  return (
    <>
      {showAnchor ? (
        <button ref={anchorRef} type="button" data-testid="anchor">
          Open
        </button>
      ) : null}
      <button type="button" onClick={() => setShowAnchor(false)}>
        Remove anchor
      </button>
      <AnchoredActionPopover
        open={open}
        anchorRef={anchorRef}
        title="Move task"
        description="Choose how to enter the next step."
        body={body}
        footer={footer}
        align={align}
        onOpenChange={setOpen}
        onDismiss={onDismiss}
      />
    </>
  );
}

describe("AnchoredActionPopover", () => {
  afterEach(cleanup);

  it("aligns to the anchor end by default", () => {
    render(<Harness />);

    expect(screen.getByRole("dialog", { name: "Move task" }).getAttribute("data-align")).toBe(
      "end",
    );
  });

  it("honors an explicit alignment override", () => {
    render(<Harness align="center" />);

    expect(screen.getByRole("dialog", { name: "Move task" }).getAttribute("data-align")).toBe(
      "center",
    );
  });

  it("focuses the first form control and returns focus to the live anchor", async () => {
    render(<Harness />);

    await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("First field")));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("anchor")));
  });

  it("dismisses when the anchor disappears", async () => {
    const onDismiss = vi.fn();
    render(<Harness onDismiss={onDismiss} />);
    fireEvent.click(screen.getByRole("button", { name: "Remove anchor" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Move task" })).toBeNull();
      expect(onDismiss).toHaveBeenCalledTimes(1);
    });
  });

  it("keeps the body scrollable and the footer outside the scroll region", () => {
    render(<Harness />);
    const dialog = screen.getByRole("dialog", { name: "Move task" });
    expect(dialog.className).toContain("overflow-hidden");
    expect(dialog.querySelector('[data-slot="anchored-action-body"]')?.className).toContain(
      "overflow-y-auto",
    );
    expect(dialog.querySelector('[data-slot="anchored-action-footer"]')?.className).toContain(
      "shrink-0",
    );
  });

  it("attaches an owner ref so hover controllers keep the anchored surface open", () => {
    const contentRef = createRef<HTMLDivElement>();
    const anchorRef = createRef<HTMLButtonElement>();
    render(
      <>
        <button ref={anchorRef} type="button">
          Anchor
        </button>
        <AnchoredActionPopover
          open
          anchorRef={anchorRef}
          contentRef={contentRef}
          title="Move task"
          body={<input aria-label="First field" />}
          onOpenChange={vi.fn()}
        />
      </>,
    );

    expect(contentRef.current).toBe(screen.getByRole("dialog", { name: "Move task" }));
  });
});
