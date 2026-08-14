"use client";

import { IconRoute } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import {
  buildRemoteRoleRows,
  type RemoteRoleDetailsStatus,
  type RemoteRoleKey,
  type RemoteRoleRow,
} from "./remote-role-details-model";

type RemoteRoleStatus = RemoteRoleDetailsStatus & { repository_name?: string };

const roleLabelKeys: Record<RemoteRoleKey, string> = {
  action_head: "task:remoteRoleActionHead",
  tracking_upstream: "task:remoteRoleTrackingUpstream",
  comparison_target: "task:remoteRoleComparisonTarget",
};

const stateLabelKeys: Record<string, string> = {
  absent: "task:remoteRoleStateAbsent",
  ambiguous: "task:remoteRoleStateAmbiguous",
  present: "task:remoteRoleStatePresent",
  resolved: "task:remoteRoleStateResolved",
  unknown: "task:remoteRoleStateUnknown",
  unresolved: "task:remoteRoleStateUnresolved",
};

function stateClass(state: string): string {
  if (state === "present" || state === "resolved") {
    return "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300";
  }
  if (state === "ambiguous" || state === "unresolved") {
    return "bg-amber-500/10 text-amber-700 dark:text-amber-300";
  }
  return "bg-muted text-muted-foreground";
}

function rowLocation(row: RemoteRoleRow, unavailable: string): string {
  const location = [row.repository, row.ref].filter(Boolean).join(" · ");
  return location || unavailable;
}

function RoleRow({ row }: { row: RemoteRoleRow }) {
  const { t } = useTranslation();
  const stateKey = stateLabelKeys[row.state] ?? "task:remoteRoleStateUnknown";
  const unavailable = t("task:remoteRoleUnavailable");
  return (
    <div
      className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-2 rounded-md border border-border/60 px-2.5 py-2"
      data-testid={`remote-role-row-${row.role}`}
    >
      <div className="min-w-0">
        <p className="text-[11px] font-medium text-foreground">{t(roleLabelKeys[row.role])}</p>
        <p
          className="truncate text-[11px] text-muted-foreground"
          title={rowLocation(row, unavailable)}
        >
          {rowLocation(row, unavailable)}
        </p>
      </div>
      <span
        className={`rounded-full px-1.5 py-0.5 text-[10px] font-medium ${stateClass(row.state)}`}
      >
        {t(stateKey)}
      </span>
    </div>
  );
}

export function RemoteRoleDetails({
  statuses,
  repoDisplayName,
}: {
  statuses: RemoteRoleStatus[];
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const { t } = useTranslation();
  const visibleStatuses = statuses.length > 0 ? statuses : [{}];
  const hasMultipleRepositories = visibleStatuses.length > 1;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="h-11 w-11 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground md:h-7 md:w-7"
          aria-label={t("task:remoteRoles")}
          title={t("task:remoteRoles")}
          data-testid="remote-roles-trigger"
        >
          <IconRoute className="h-3.5 w-3.5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-[min(24rem,calc(100vw-1rem))] p-3"
        data-testid="remote-roles-popover"
      >
        <div className="mb-3 space-y-1">
          <p className="text-xs font-semibold text-foreground">{t("task:remoteRoles")}</p>
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t("task:remoteRolesDescription")}
          </p>
        </div>
        <div className="space-y-3">
          {visibleStatuses.map((status, index) => {
            const repositoryName = status.repository_name ?? "";
            const rows = buildRemoteRoleRows(status);
            const repositoryLabel =
              repoDisplayName?.(repositoryName) || repositoryName || t("task:repository2");
            return (
              <section
                key={`${repositoryName || "root"}-${index}`}
                className="space-y-1.5"
                data-testid={`remote-roles-repository-${repositoryName || "root"}`}
              >
                {hasMultipleRepositories && (
                  <p className="truncate text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                    {repositoryLabel}
                  </p>
                )}
                <div className="space-y-1.5">
                  {rows.map((row) => (
                    <RoleRow key={row.role} row={row} />
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}
