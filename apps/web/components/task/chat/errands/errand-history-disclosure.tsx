"use client";

import type { ReactNode } from "react";
import { IconChevronRight, IconClockHour4 } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { cn } from "@/lib/utils";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";

/** Historical events stay quiet until the reader asks for their recorded details. */
export function ErrandHistoryDisclosure({
  testId,
  errandId,
  summary,
  children,
}: {
  testId: string;
  errandId?: string | null;
  summary: ReactNode;
  children: ReactNode;
}) {
  return (
    <Collapsible
      data-testid={testId}
      data-errand-id={errandId ?? undefined}
      className="w-full min-w-0 text-xs"
    >
      <CollapsibleTrigger
        data-testid={`${testId}-disclosure`}
        className={cn(
          "group flex w-full min-w-0 cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-muted-foreground hover:bg-muted/30",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
          ERRAND_TOUCH_TARGET,
        )}
      >
        <IconChevronRight
          aria-hidden
          className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60 group-data-[state=open]:rotate-90"
        />
        <IconClockHour4 aria-hidden className="h-3.5 w-3.5 shrink-0" />
        <span className="min-w-0 break-words [overflow-wrap:anywhere]">{summary}</span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mx-2 space-y-2 border-t border-border/40 px-2.5 py-2">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}
