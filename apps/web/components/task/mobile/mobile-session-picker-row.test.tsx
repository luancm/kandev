import { render } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { MobileSessionPickerRow } from "./mobile-session-picker-row";

vi.mock("./mobile-sessions-section", () => ({
  MobileSessionsPicker: ({ sessionId }: { sessionId: string | null }) => (
    <button type="button" data-testid="mobile-sessions-pill" data-session-id={sessionId} />
  ),
}));

it.each(["session-parent", null])("shows the ordinary phone picker for %s", (sessionId) => {
  const { getByTestId, unmount } = render(
    <MobileSessionPickerRow taskId="task-1" sessionId={sessionId} />,
  );
  expect(getByTestId("mobile-sessions-pill").getAttribute("data-session-id")).toBe(sessionId);
  unmount();
});
