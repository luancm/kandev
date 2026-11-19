"use client";

import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ErrandHeaderActionButton, ErrandHeaderActionsMenu } from "./errand-header-actions";
import {
  BackToParentButton,
  ErrandRecoveryHeaderNotice,
  ErrandMetaRow,
  ErrandObjectiveDisclosure,
} from "./errand-header-parts";
import { useErrandViewModel, type ErrandViewModel } from "./use-errand-view-model";

type HeaderProps = { model: ErrandViewModel; onBack?: () => void };

function ErrandHeaderDesktop({ model, onBack }: HeaderProps) {
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
          <BackToParentButton model={model} variant="desktop" onBack={onBack} />
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
        <ErrandHeaderActionButton errand={model.errand} sessionId={model.errandId} />
      </div>
      <ErrandMetaRow model={model} showWorkspace />
      <ErrandRecoveryHeaderNotice errand={model.errand} />
      <ErrandObjectiveDisclosure model={model} includeWorkspace={false} />
    </header>
  );
}

/** Phone form: back to the parent, the title, and a visible actions menu. */
function ErrandHeaderPhone({ model, onBack }: HeaderProps) {
  return (
    <header
      data-testid="errand-header"
      data-layout="phone"
      className="shrink-0 space-y-1 border-b border-border/60 bg-card px-1 pb-2 text-xs"
    >
      <div className="flex min-w-0 items-center gap-1">
        <BackToParentButton model={model} variant="phone" onBack={onBack} />
        <h2
          className="min-w-0 flex-1 truncate text-center text-sm font-semibold text-foreground"
          title={model.title}
        >
          {model.title}
        </h2>
        <ErrandHeaderActionsMenu errand={model.errand} sessionId={model.errandId} />
      </div>
      <div className="space-y-1 px-2">
        <ErrandMetaRow model={model} showWorkspace={false} />
        <ErrandRecoveryHeaderNotice errand={model.errand} />
        <ErrandObjectiveDisclosure model={model} includeWorkspace />
      </div>
    </header>
  );
}

/** Header above an Errand's transcript. */
export function ErrandHeader({
  errandId,
  taskId,
  onBack,
}: {
  errandId: string;
  taskId: string;
  onBack?: () => void;
}) {
  const model = useErrandViewModel(errandId, taskId);
  const { isMobile } = useResponsiveBreakpoint();
  return isMobile ? (
    <ErrandHeaderPhone model={model} onBack={onBack} />
  ) : (
    <ErrandHeaderDesktop model={model} onBack={onBack} />
  );
}
