"use client";

import { useId } from "react";
import { IconLoader2, IconPlayerStop, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { DrawerClose, DrawerDescription, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useErrandActions } from "@/hooks/domains/session/use-errand-actions";
import { useNow } from "@/hooks/use-now";
import type { Errand } from "@/lib/types/errands";
import { orderErrandPanel } from "@/lib/errand-panel-order";
import { cn } from "@/lib/utils";
import { selectSessionTabTitle } from "../../session-tab-title";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";
import { ErrandsPanelRow } from "./errands-panel-row";

const CLOCK_TICK_MS = 30_000;

export type ErrandsPanelProps = {
  taskId: string;
  errands: Errand[];
  errandsByParent: Record<string, Errand[]>;
  liveCount: number;
  isLoaded: boolean;
  /** Bottom-drawer form for phones and coarse pointers; popover form otherwise. */
  drawer: boolean;
  /** Closes the surface hosting the panel. */
  onClose: () => void;
};

function ParentHeading({ parentSessionId, id }: { parentSessionId: string; id: string }) {
  const { t } = useTranslation();
  const title = useAppStore((state) => selectSessionTabTitle(state, parentSessionId));
  return (
    <h3 id={id} className="truncate px-0.5 text-[11px] font-medium text-muted-foreground">
      {title || t("task:errandParentFallback")}
    </h3>
  );
}

export function ErrandsParentGroup({
  parentSessionId,
  errands,
  taskId,
  nowMs,
  drawer,
  onClose,
}: {
  parentSessionId: string;
  errands: Errand[];
  taskId: string;
  nowMs: number;
  drawer: boolean;
  onClose: () => void;
}) {
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      data-testid="errands-panel-group"
      className="flex flex-col gap-1"
    >
      <ParentHeading parentSessionId={parentSessionId} id={headingId} />
      <ul className="flex flex-col gap-1.5">
        {errands.map((errand) => (
          <ErrandsPanelRow
            key={errand.errand_id}
            errand={errand}
            taskId={taskId}
            nowMs={nowMs}
            drawer={drawer}
            onOpened={onClose}
          />
        ))}
      </ul>
    </section>
  );
}

export function ErrandsStopAllButton({ taskId, drawer }: { taskId: string; drawer: boolean }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { cancelAllErrands, isPending } = useErrandActions();
  const pending = isPending("cancel_all", taskId);
  return (
    <Button
      type="button"
      variant="destructive"
      size="sm"
      disabled={pending}
      data-testid="errands-stop-all"
      onClick={() => {
        cancelAllErrands(taskId).catch(() => {
          toast({ title: t("task:errandsStopAllFailed"), variant: "error" });
        });
      }}
      className={cn("w-full cursor-pointer justify-center gap-1 text-xs", drawer && "min-h-11")}
    >
      {pending ? (
        <IconLoader2 aria-hidden className="h-3.5 w-3.5 animate-spin" />
      ) : (
        <IconPlayerStop aria-hidden className="h-3.5 w-3.5" />
      )}
      {t("task:errandsStopAll")}
    </Button>
  );
}

function PanelList(props: ErrandsPanelProps & { nowMs: number }) {
  const { t } = useTranslation();
  const { errands, taskId, nowMs, drawer, onClose, isLoaded } = props;
  const ordered = orderErrandPanel(errands);
  return (
    <div
      data-testid="errands-panel-list"
      data-vaul-no-drag={drawer ? true : undefined}
      className={cn(
        "flex flex-col gap-3 overflow-y-auto",
        drawer ? "min-h-0 flex-1 px-4 pb-3" : "max-h-64",
      )}
    >
      {(
        [
          ["active", t("github:active")],
          ["finished", t("automations:completed")],
        ] as const
      ).map(
        ([section, label]) =>
          ordered[section].length > 0 && (
            <section key={section} aria-label={label} className="flex flex-col gap-2">
              <h2 className="text-xs font-semibold text-muted-foreground">{label}</h2>
              {ordered[section].map(({ parentId, errands: group }) => (
                <ErrandsParentGroup
                  key={parentId}
                  parentSessionId={parentId}
                  errands={group}
                  taskId={taskId}
                  nowMs={nowMs}
                  drawer={drawer}
                  onClose={onClose}
                />
              ))}
            </section>
          ),
      )}
      {!isLoaded && (
        <p
          role="status"
          data-testid="errands-panel-loading"
          className="flex items-center gap-1.5 px-0.5 text-[11px] text-muted-foreground"
        >
          <IconLoader2 aria-hidden className="h-3 w-3 animate-spin" />
          {t("task:errandsPanelLoading")}
        </p>
      )}
    </div>
  );
}

function DrawerPanelHeader() {
  const { t } = useTranslation();
  return (
    <DrawerHeader className="shrink-0 flex-row items-center justify-between gap-3 text-left">
      <div className="min-w-0">
        <DrawerTitle>{t("task:errandsPanelTitle")}</DrawerTitle>
        <DrawerDescription>{t("task:errandsPanelDescription")}</DrawerDescription>
      </div>
      <DrawerClose asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("task:errandsPanelClose")}
          className={ERRAND_TOUCH_TARGET}
        >
          <IconX aria-hidden="true" />
        </Button>
      </DrawerClose>
    </DrawerHeader>
  );
}

function PopoverPanelHeader({ total }: { total: number }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 border-b border-border pb-2">
      <span className="text-xs font-semibold text-foreground">{t("task:errandsPanelTitle")}</span>
      <Badge variant="outline" className="text-[10px]">
        {t("task:errandsPanelTotal", { count: total })}
      </Badge>
    </div>
  );
}

/**
 * Errands of a task grouped by the session that started them, with **Stop
 * all errands** pinned below the list while any errand is live. Mounted only
 * while its popover or drawer is open, so its clock stops when it closes.
 */
export function ErrandsPanel(props: ErrandsPanelProps) {
  const { taskId, errands, liveCount, drawer } = props;
  const nowMs = useNow(CLOCK_TICK_MS);
  const footer = liveCount > 0 && <ErrandsStopAllButton taskId={taskId} drawer={drawer} />;
  if (drawer) {
    return (
      <div data-testid="errands-panel" className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <DrawerPanelHeader />
        <PanelList {...props} nowMs={nowMs} />
        {footer && (
          <div className="shrink-0 border-t border-border p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {footer}
          </div>
        )}
      </div>
    );
  }
  return (
    <div data-testid="errands-panel" className="flex flex-col gap-2">
      <PopoverPanelHeader total={errands.length} />
      <PanelList {...props} nowMs={nowMs} />
      {footer && <div className="border-t border-border pt-2">{footer}</div>}
    </div>
  );
}
