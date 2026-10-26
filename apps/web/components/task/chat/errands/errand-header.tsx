"use client";

import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ErrandHeaderActionButton, ErrandHeaderActionsMenu } from "./errand-header-actions";
import {
  BackToParentButton,
  ErrandMetaRow,
  ErrandObjectiveDisclosure,
} from "./errand-header-parts";
import { useErrandViewModel, type ErrandViewModel } from "./use-errand-view-model";

function ErrandHeaderDesktop({ model }: { model: ErrandViewModel }) {
  const { t } = useTranslation();
  return (
    <header
      data-testid="errand-header"
      data-layout="desktop"
      className="shrink-0 space-y-1 border-b border-border/60 bg-card px-2.5 py-1.5 text-xs"
    >
      <div className="flex min-w-0 items-center gap-2">
        <nav
          aria-label={t("task:errandBreadcrumbAria")}
          className="flex min-w-0 flex-1 items-center gap-1"
        >
          <BackToParentButton model={model} variant="desktop" />
          <span aria-hidden="true" className="shrink-0 text-muted-foreground/50">
            /
          </span>
          <span className="shrink-0 text-muted-foreground">{t("task:errandHeaderPrefix")}</span>
          <h2
            className="min-w-0 truncate text-xs font-semibold text-foreground"
            title={model.title}
          >
            {model.title}
          </h2>
        </nav>
        <ErrandHeaderActionButton errand={model.errand} sessionId={model.sessionId} />
      </div>
      <ErrandMetaRow model={model} showWorkspace />
      <ErrandObjectiveDisclosure model={model} includeWorkspace={false} />
    </header>
  );
}

/** Phone form: back to the parent, the title, and a visible actions menu. */
function ErrandHeaderPhone({ model }: { model: ErrandViewModel }) {
  return (
    <header
      data-testid="errand-header"
      data-layout="phone"
      className="shrink-0 space-y-1 border-b border-border/60 bg-card px-1 pb-2 text-xs"
    >
      <div className="flex min-w-0 items-center gap-1">
        <BackToParentButton model={model} variant="phone" />
        <h2
          className="min-w-0 flex-1 truncate text-center text-sm font-semibold text-foreground"
          title={model.title}
        >
          {model.title}
        </h2>
        <ErrandHeaderActionsMenu errand={model.errand} sessionId={model.sessionId} />
      </div>
      <div className="space-y-1 px-2">
        <ErrandMetaRow model={model} showWorkspace={false} />
        <ErrandObjectiveDisclosure model={model} includeWorkspace />
      </div>
    </header>
  );
}

/**
 * Header above an errand's transcript. Renders nothing for ordinary sessions
 * or while the errands flag is off.
 */
export function ErrandHeader({
  sessionId,
  taskId,
}: {
  sessionId: string | null | undefined;
  taskId: string | null | undefined;
}) {
  const model = useErrandViewModel(sessionId, taskId);
  const { isMobile } = useResponsiveBreakpoint();
  if (!model) return null;
  return isMobile ? <ErrandHeaderPhone model={model} /> : <ErrandHeaderDesktop model={model} />;
}
