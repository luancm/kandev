"use client";

import type { ReactNode } from "react";
import { IconChevronRight } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

/**
 * An errand nested under its parent in the phone sessions drawer: one 44px
 * target showing its title and status. Errands have no per-row menu here;
 * Stop and Send result live in the errand view's own actions menu.
 */
export function MobileErrandSessionRow({
  sessionId,
  title,
  status,
  isActive,
  onSelect,
}: {
  sessionId: string;
  title: string;
  status: ReactNode;
  isActive: boolean;
  onSelect: (sessionId: string) => void;
}) {
  return (
    <button
      type="button"
      aria-current={isActive ? "true" : undefined}
      onClick={() => onSelect(sessionId)}
      data-testid={`mobile-errand-row-${sessionId}`}
      className={cn(
        "flex min-h-11 w-full cursor-pointer select-none items-center gap-2 rounded-md border px-2 py-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        isActive ? "border-primary/50 bg-card" : "border-transparent hover:bg-muted",
      )}
    >
      <span className="min-w-0 flex-1 truncate text-sm" title={title}>
        {title}
      </span>
      {status}
      <IconChevronRight aria-hidden className="h-4 w-4 shrink-0 text-muted-foreground" />
    </button>
  );
}
