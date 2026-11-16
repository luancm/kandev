"use client";

import { useErrandViewModel } from "../chat/errands/use-errand-view-model";
import { MobileSessionsPicker } from "./mobile-sessions-section";

/**
 * The sessions-pill row above the phone chat. An errand view replaces it with
 * its own header (back to the parent session and an actions menu).
 */
export function MobileSessionPickerRow({
  taskId,
  sessionId,
}: {
  taskId: string;
  sessionId: string | null;
}) {
  const errandView = useErrandViewModel(sessionId, taskId);
  if (errandView) return null;
  return (
    <div className="flex items-center px-1 py-2" data-testid="mobile-session-picker-row">
      <MobileSessionsPicker taskId={taskId} sessionId={sessionId} fullWidth />
    </div>
  );
}
