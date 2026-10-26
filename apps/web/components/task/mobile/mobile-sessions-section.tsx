"use client";

import { Fragment, memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconPlus, IconStar } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

import { AgentLogo } from "@/components/agent-logo";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useTaskErrands } from "@/hooks/domains/session/use-task-errands";
import { ErrandStatusLabel } from "../chat/errands/errand-status";
import type { Errand } from "@/lib/types/errands";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useSessionActions } from "@/hooks/domains/session/use-session-actions";
import { NewSessionDialog, type HandoffPreset } from "../new-session-dialog";
import { MobilePillButton } from "./mobile-pill-button";
import { MobilePickerSheet } from "./mobile-picker-sheet";
import { formatTaskSessionStateLabel } from "@/lib/ui/state-labels";
import { getSessionStateIcon } from "@/lib/ui/state-icons";
import { repositorySlug } from "@/lib/repository-slug";
import { useSessionPendingInput, type PendingInput } from "@/hooks/use-task-pending-input";
import type { ForegroundActivity, TaskSession, TaskSessionState } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { SessionActionsMenu } from "./session-actions-menu";
import { MobileSessionDeleteConfirmation } from "./mobile-session-delete-confirmation";
import { MobileErrandSessionRow } from "./mobile-errand-session-row";
import { nestSessionRows } from "./mobile-session-nesting";

type SessionRow = {
  id: string;
  agentName: string | null;
  agentLabel: string;
  repositoryLabel: string | null;
  state: TaskSessionState | null;
  foregroundActivity: ForegroundActivity | null;
  /** True when the session is waiting on the operator to notice, not to act —
   *  a settled session with a positively-sampled background process still
   *  live (spec: docs/specs/disambiguate-waiting/spec.md). */
  parkedOnBackgroundWork: boolean;
  isPrimary: boolean;
  /** The session's role is `errand`, independent of the errands flag. */
  isErrand: boolean;
  /** An errand shown nested under its parent (errands flag on). */
  nested: boolean;
  parentSessionId: string | null;
  /** Position among the top-level sessions; 0 for a nested errand. */
  index: number;
  startedAt: string;
};

/** User-supplied session name wins over the derived profile label, like the desktop tab title. */
function sessionAgentLabel(session: TaskSession, profile: AgentProfileOption | undefined): string {
  const labelParts = profile?.label.split(" • ") ?? [];
  return session.name || labelParts[1] || labelParts[0] || profile?.label || t("task:agent");
}

function buildSessionRows(
  sessions: TaskSession[],
  agentProfiles: AgentProfileOption[],
  primarySessionId: string | null | undefined,
  repositoryLabelsById: ReadonlyMap<string, string>,
  errandsEnabled: boolean,
): SessionRow[] {
  const sorted = [...sessions].sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
  );
  let ordinal = 0;
  return sorted.map((s) => {
    const isErrand = s.role === "errand";
    const nested = errandsEnabled && isErrand;
    const profile = agentProfiles.find((p) => p.id === s.agent_profile_id);
    return {
      id: s.id,
      agentName: profile?.agent_name ?? null,
      agentLabel: sessionAgentLabel(s, profile),
      repositoryLabel: s.repository_id ? (repositoryLabelsById.get(s.repository_id) ?? null) : null,
      state: (s.state as TaskSessionState | undefined) ?? null,
      foregroundActivity: s.foreground_activity ?? null,
      parkedOnBackgroundWork: !!s.parked_on_background_work,
      isPrimary: primarySessionId ? s.id === primarySessionId : !!s.is_primary,
      isErrand,
      nested,
      parentSessionId: s.parent_session_id ?? null,
      index: nested ? 0 : ++ordinal,
      startedAt: s.started_at,
    };
  });
}

// The mobile sessions section reads the per-session substate through the same
// shared vocabulary as the desktop session switcher and the reopen menu
// `getSessionStateIcon` carries background-running
// as a spinner distinct — by SHAPE and MOTION, not hue alone — from generating
// (a static dot) and from done (a check), so the three read apart even in a
// grayscale scan. The label reinforces the distinction in words: while spawned
// background work runs, the row reads "Working in background" rather than the
// bare "Running", so background is never confused with generating.
// A catalog key, not copy: `t()` at module scope would freeze at the boot locale.
const BACKGROUND_RUNNING_LABEL_KEY = "task:workingInBackground";

function sessionStateLabel(
  state: TaskSessionState,
  foregroundActivity: ForegroundActivity | null,
  pending: PendingInput,
  parkedOnBackgroundWork: boolean,
): string {
  const canRequestInput = state === "RUNNING" || state === "WAITING_FOR_INPUT";
  if (canRequestInput && pending.permission) return t("task:permissionRequested");
  if (canRequestInput && pending.clarification) {
    return formatTaskSessionStateLabel("WAITING_FOR_INPUT");
  }
  if (canRequestInput && foregroundActivity === "background") {
    return t(BACKGROUND_RUNNING_LABEL_KEY);
  }
  if (canRequestInput && parkedOnBackgroundWork) {
    return t(BACKGROUND_RUNNING_LABEL_KEY);
  }
  return formatTaskSessionStateLabel(state);
}

function StateBadge({
  sessionId,
  state,
  foregroundActivity,
  parkedOnBackgroundWork,
  testId,
}: {
  sessionId: string;
  state: TaskSessionState | null;
  foregroundActivity: ForegroundActivity | null;
  parkedOnBackgroundWork: boolean;
  testId?: string;
}) {
  const pending = useSessionPendingInput(sessionId);
  if (!state) return null;
  const label = sessionStateLabel(state, foregroundActivity, pending, parkedOnBackgroundWork);
  return (
    <span
      data-testid={testId}
      title={label}
      className="flex items-center gap-1 whitespace-nowrap text-[10px] font-medium leading-none text-muted-foreground shrink-0"
    >
      {getSessionStateIcon(state, "h-3 w-3 shrink-0", {
        foregroundActivity,
        hasPendingClarification: pending.clarification,
        hasPendingPermission: pending.permission,
        parkedOnBackgroundWork,
      })}
      {label}
    </span>
  );
}

function SessionIdentity({ row }: { row: SessionRow }) {
  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <span className="truncate text-sm">{row.agentLabel}</span>
      {row.repositoryLabel && (
        <span
          className="truncate text-[11px] text-muted-foreground"
          data-testid={`mobile-session-repository-${row.id}`}
        >
          {row.repositoryLabel}
        </span>
      )}
    </div>
  );
}

/** Connects one session's menu and delete confirmation to its shared lifecycle actions. */
function SessionRowTrailingActions({
  row,
  taskId,
  totalSessions,
  isConfirming,
  onAskDelete,
  onCancelDelete,
  onHandoffProfile,
  actions,
}: {
  row: SessionRow;
  taskId: string;
  totalSessions: number;
  isConfirming: boolean;
  onAskDelete: () => void;
  onCancelDelete: () => void;
  onHandoffProfile: (profileId: string) => void;
  actions: ReturnType<typeof useSessionActions>;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const triggerRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <MobileSessionDeleteConfirmation
        focusReturnRef={triggerRef}
        open={isConfirming}
        targetKey={`${taskId}:${row.id}`}
        isPrimary={row.isPrimary}
        isOnlySession={totalSessions === 1}
        targetName={row.agentLabel}
        onCancel={onCancelDelete}
        onClose={onCancelDelete}
        onConfirm={() => void actions.remove()}
      />
      {(!isConfirming || isMobile) && (
        <SessionActionsMenu
          triggerRef={triggerRef}
          taskId={taskId}
          state={row.state}
          isPrimary={row.isPrimary}
          isErrand={row.isErrand}
          onSetPrimary={() => void actions.setPrimary()}
          onStop={() => void actions.stop()}
          onResume={() => void actions.resume()}
          onRestart={() => void actions.restart()}
          isRestarting={actions.isRestarting}
          onAskDelete={onAskDelete}
          onHandoffProfile={onHandoffProfile}
        />
      )}
    </>
  );
}

function SessionRowItem({
  row,
  taskId,
  isActive,
  totalSessions,
  isConfirming,
  onAskDelete,
  onCancelDelete,
  onSelect,
}: {
  row: SessionRow;
  taskId: string;
  isActive: boolean;
  totalSessions: number;
  isConfirming: boolean;
  onAskDelete: () => void;
  onCancelDelete: () => void;
  onSelect: (sessionId: string) => void;
}) {
  const [handoffOpen, setHandoffOpen] = useState(false);
  const [handoffPreset, setHandoffPreset] = useState<HandoffPreset | null>(null);
  const actions = useSessionActions({ sessionId: row.id, taskId });
  const showBadges = totalSessions > 1;
  const handleHandoffProfile = useCallback(
    (profileId: string) => {
      setHandoffPreset({ sourceSessionId: row.id, targetProfileId: profileId });
      setHandoffOpen(true);
    },
    [row.id],
  );

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        aria-current={isActive ? "true" : undefined}
        onClick={() => onSelect(row.id)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onSelect(row.id);
          }
        }}
        data-testid={`mobile-session-row-${row.id}`}
        className={`flex items-center gap-2 border px-2 py-2 rounded-md cursor-pointer select-none ${
          isActive ? "border-primary/50 bg-card" : "border-transparent hover:bg-muted"
        }`}
      >
        {row.isPrimary && showBadges && (
          <IconStar className="h-3.5 w-3.5 fill-foreground/60 stroke-0 shrink-0" />
        )}
        {showBadges && (
          <span className="text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5 shrink-0">
            {row.index}
          </span>
        )}
        {row.agentName && <AgentLogo agentName={row.agentName} size={16} className="shrink-0" />}
        <SessionIdentity row={row} />
        <StateBadge
          sessionId={row.id}
          state={row.state}
          foregroundActivity={row.foregroundActivity}
          parkedOnBackgroundWork={row.parkedOnBackgroundWork}
          testId={`mobile-session-state-${row.id}`}
        />
        <SessionRowTrailingActions
          row={row}
          taskId={taskId}
          totalSessions={totalSessions}
          isConfirming={isConfirming}
          onAskDelete={onAskDelete}
          onCancelDelete={onCancelDelete}
          onHandoffProfile={handleHandoffProfile}
          actions={actions}
        />
      </div>
      {handoffPreset && (
        <NewSessionDialog
          open={handoffOpen}
          onOpenChange={(open) => {
            setHandoffOpen(open);
            if (!open) setHandoffPreset(null);
          }}
          taskId={taskId}
          handoff={handoffPreset}
        />
      )}
    </>
  );
}

function useSessionRows(taskId: string | null) {
  // `buildSessionRows` resolves its agent-name fallback through the module-level
  // `t`, so the language has to reach the memo below for that label to follow a
  // runtime locale switch.
  const { i18n } = useTranslation();
  const errandsEnabled = useFeature("errands");
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const repositoriesByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  const primarySessionId = useAppStore((s) => {
    if (!taskId) return null;
    const task = s.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.primarySessionId ?? null;
  });
  const { sessions, isLoading } = useTaskSessions(taskId);
  const repositoryLabelsById = useMemo(() => {
    const sessionRepositoryIds = new Set(
      sessions.flatMap((session) => (session.repository_id ? [session.repository_id] : [])),
    );
    if (sessionRepositoryIds.size <= 1) return new Map<string, string>();
    const labels = new Map<string, string>();
    for (const repository of Object.values(repositoriesByWorkspace).flat()) {
      if (sessionRepositoryIds.has(repository.id)) {
        labels.set(repository.id, repositorySlug(repository));
      }
    }
    return labels;
  }, [repositoriesByWorkspace, sessions]);
  const rows = useMemo(
    () =>
      buildSessionRows(
        sessions,
        agentProfiles,
        primarySessionId,
        repositoryLabelsById,
        errandsEnabled,
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- i18n.language is the locale trigger
    [
      sessions,
      agentProfiles,
      primarySessionId,
      repositoryLabelsById,
      errandsEnabled,
      i18n.language,
    ],
  );
  return { rows, isLoading, primarySessionId };
}

/** Errand sessions of a task by session ID; empty while the errands flag is off. */
function useErrandsBySessionId(taskId: string | null): ReadonlyMap<string, Errand> {
  const { errands } = useTaskErrands(taskId);
  return useMemo(() => new Map(errands.map((errand) => [errand.errand_id, errand])), [errands]);
}

function SessionRowsTree({
  rows,
  taskId,
  activeSessionId,
  confirmDeleteSessionId,
  onAskDelete,
  onCancelDelete,
  onSelect,
}: {
  rows: SessionRow[];
  taskId: string;
  activeSessionId: string | null;
  confirmDeleteSessionId: string | null;
  onAskDelete: (sessionId: string) => void;
  onCancelDelete: () => void;
  onSelect: (sessionId: string) => void;
}) {
  const { t } = useTranslation();
  const errandsById = useErrandsBySessionId(taskId);
  const groups = useMemo(
    () =>
      nestSessionRows(
        rows.map((row) => {
          const parent = errandsById.get(row.id)?.parent_session_id;
          return parent ? { ...row, parentSessionId: parent } : row;
        }),
      ),
    [rows, errandsById],
  );
  const topLevelCount = rows.filter((row) => !row.nested).length;
  const renderErrand = (row: SessionRow) => {
    const errand = errandsById.get(row.id);
    return (
      <MobileErrandSessionRow
        key={row.id}
        sessionId={row.id}
        title={errand?.title || row.agentLabel}
        isActive={row.id === activeSessionId}
        onSelect={onSelect}
        status={
          errand ? (
            <ErrandStatusLabel status={errand.status} />
          ) : (
            <StateBadge
              sessionId={row.id}
              state={row.state}
              foregroundActivity={row.foregroundActivity}
              parkedOnBackgroundWork={row.parkedOnBackgroundWork}
            />
          )
        }
      />
    );
  };
  return groups.map(({ row, children }) => (
    <Fragment key={row.id}>
      {row.nested ? (
        renderErrand(row)
      ) : (
        <SessionRowItem
          row={row}
          taskId={taskId}
          isActive={row.id === activeSessionId}
          totalSessions={topLevelCount}
          isConfirming={row.id === confirmDeleteSessionId}
          onAskDelete={() => onAskDelete(row.id)}
          onCancelDelete={onCancelDelete}
          onSelect={onSelect}
        />
      )}
      {children.length > 0 && (
        <div
          role="group"
          aria-label={t("task:errandsOfSession", { parent: row.agentLabel })}
          data-testid={`mobile-errand-group-${row.id}`}
          className="ml-4 flex flex-col gap-0.5 border-l border-border/60 pl-2"
        >
          {children.map(renderErrand)}
        </div>
      )}
    </Fragment>
  ));
}

const MobileSessionsList = memo(function MobileSessionsList({
  taskId,
  activeSessionId,
  open,
  onClose,
}: {
  taskId: string | null;
  activeSessionId: string | null;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const { rows, isLoading } = useSessionRows(taskId);
  const [launchOpen, setLaunchOpen] = useState(false);
  const [confirmDeleteSessionId, setConfirmDeleteSessionId] = useState<string | null>(null);

  useEffect(() => {
    if (!open) setConfirmDeleteSessionId(null);
  }, [open]);

  const handleSelect = useCallback(
    (sessionId: string) => {
      if (!taskId) return;
      setConfirmDeleteSessionId(null);
      setActiveSession(taskId, sessionId);
      onClose();
    },
    [taskId, setActiveSession, onClose],
  );

  if (!taskId) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-6 text-center">
        {t("task:noActiveTask")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2 px-1">
      <div className="flex items-center justify-between px-1">
        <span className="text-xs font-medium text-muted-foreground">
          {t("task:sessionCount", { count: rows.filter((row) => !row.nested).length })}
        </span>
        <Button
          size="sm"
          variant="outline"
          className="h-7 gap-1 cursor-pointer"
          onClick={() => setLaunchOpen(true)}
          data-testid="mobile-launch-session"
        >
          <IconPlus className="h-4 w-4" />
          {t("task:newSession")}
        </Button>
      </div>
      <div className="flex flex-col gap-0.5">
        {isLoading && rows.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            {t("task:loadingSessions")}
          </div>
        )}
        {!isLoading && rows.length === 0 && (
          <div className="text-xs text-muted-foreground px-2 py-4 text-center">
            {t("task:noSessionsYetLaunchOneTo")}
          </div>
        )}
        <SessionRowsTree
          rows={rows}
          taskId={taskId}
          activeSessionId={activeSessionId}
          confirmDeleteSessionId={confirmDeleteSessionId}
          onAskDelete={setConfirmDeleteSessionId}
          onCancelDelete={() => setConfirmDeleteSessionId(null)}
          onSelect={handleSelect}
        />
      </div>
      {launchOpen && (
        <NewSessionDialog open={launchOpen} onOpenChange={setLaunchOpen} taskId={taskId} />
      )}
    </div>
  );
});

function useActiveSessionPillLabel(
  taskId: string | null,
  sessionId: string | null | undefined,
): {
  label: string;
  count: string | undefined;
  agentName: string | null;
  effectiveSessionId: string | null;
  ariaLabel: string;
} {
  const { t } = useTranslation();
  const storedActiveSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { rows } = useSessionRows(taskId);
  const effectiveSessionId = sessionId === undefined ? storedActiveSessionId : sessionId;
  const activeRow = rows.find((r) => r.id === effectiveSessionId);
  const total = rows.filter((r) => !r.nested).length;
  const idx = activeRow?.index;
  let count: string | undefined;
  if (total > 1 && idx) count = `${idx}/${total}`;
  else if (total > 1) count = `${total}`;
  const agentLabel = activeRow?.agentLabel ?? t("task:sessionFallbackLabel");
  const repositoryLabel = activeRow?.repositoryLabel;
  return {
    label: repositoryLabel ? `${agentLabel} · ${repositoryLabel}` : agentLabel,
    count,
    agentName: activeRow?.agentName ?? null,
    effectiveSessionId,
    ariaLabel: repositoryLabel
      ? t("task:activeSessionRepositoryTapToSwitch", { agentLabel, repositoryLabel })
      : t("task:activeSessionTapToSwitch", { agentLabel }),
  };
}

export const MobileSessionsPicker = memo(function MobileSessionsPicker({
  taskId,
  sessionId,
  compact,
  fullWidth,
}: {
  taskId: string | null;
  sessionId?: string | null;
  compact?: boolean;
  fullWidth?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { label, count, agentName, effectiveSessionId, ariaLabel } = useActiveSessionPillLabel(
    taskId,
    sessionId,
  );
  if (!taskId) return null;
  return (
    <>
      <MobilePillButton
        icon={
          agentName ? (
            <span
              className="flex shrink-0 items-center"
              data-testid="mobile-session-agent-icon"
              data-agent-name={agentName}
            >
              <AgentLogo agentName={agentName} size={16} className="shrink-0" />
            </span>
          ) : undefined
        }
        label={label}
        count={count}
        compact={compact}
        fullWidth={fullWidth}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="mobile-sessions-pill"
        ariaLabel={ariaLabel}
      />
      <MobilePickerSheet
        key={taskId}
        open={open}
        onOpenChange={setOpen}
        title={t("task:sessions")}
        confirmationHost
      >
        <MobileSessionsList
          taskId={taskId}
          activeSessionId={effectiveSessionId}
          open={open}
          onClose={() => setOpen(false)}
        />
      </MobilePickerSheet>
    </>
  );
});
