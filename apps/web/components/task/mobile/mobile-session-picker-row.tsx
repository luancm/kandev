"use client";

import { MobileSessionsPicker } from "./mobile-sessions-section";

/** The sessions picker above ordinary phone chat. */
export function MobileSessionPickerRow({
  taskId,
  sessionId,
}: {
  taskId: string;
  sessionId: string | null;
}) {
  return (
    <div className="flex items-center px-1 py-2" data-testid="mobile-session-picker-row">
      <MobileSessionsPicker taskId={taskId} sessionId={sessionId} fullWidth />
    </div>
  );
}
