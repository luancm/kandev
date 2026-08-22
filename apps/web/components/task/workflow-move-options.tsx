"use client";

import { useCallback, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Textarea } from "@kandev/ui/textarea";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useAppStore } from "@/components/state-provider";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { cn } from "@/lib/utils";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AvailableAgent, CapabilityStatus } from "@/lib/types/http-agents";
import { useTranslation } from "react-i18next";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

export type WorkflowMoveOptionsDraft = {
  resetContext: boolean;
  instructions: string;
  agentProfileId: string;
  model: string;
};

const EMPTY_DRAFT: WorkflowMoveOptionsDraft = {
  resetContext: false,
  instructions: "",
  agentProfileId: "",
  model: "",
};

export function workflowMoveOptionsPayload(
  draft: WorkflowMoveOptionsDraft,
): WorkflowMoveEntryOptions | undefined {
  const payload: WorkflowMoveEntryOptions = {};
  if (draft.resetContext) payload.reset_context = true;
  if (draft.instructions.trim()) payload.instructions = draft.instructions.trim();
  if (draft.agentProfileId.trim()) payload.agent_profile_id = draft.agentProfileId.trim();
  if (draft.model.trim()) payload.model = draft.model.trim();
  return Object.keys(payload).length > 0 ? payload : undefined;
}

function capabilityReason(
  t: ReturnType<typeof useTranslation>["t"],
  status: CapabilityStatus | undefined,
  error: string | undefined,
): string | undefined {
  switch (status) {
    case "auth_required":
      return error || t("agents:authenticationRequired");
    case "not_installed":
      return error || t("task:agentCliNotInstalled");
    case "failed":
      return error || t("task:agentProbeFailed");
    case "probing":
    case "not_configured":
      return t("task:workflowMoveCapabilitiesLoading");
    default:
      return undefined;
  }
}

function profileCapabilityStatus(
  profile: AgentProfileOption,
  availableAgent: AvailableAgent | undefined,
): CapabilityStatus | undefined {
  return profile.capability_status ?? availableAgent?.model_config.status;
}

function profileDisabledReason(
  t: ReturnType<typeof useTranslation>["t"],
  profile: AgentProfileOption,
  availableAgent: AvailableAgent | undefined,
  capabilitiesLoaded: boolean,
): string | undefined {
  if (!capabilitiesLoaded) return t("task:workflowMoveCapabilitiesLoading");
  if (profile.enabled === false) return t("task:workflowMoveProfileUnavailable");
  if (!availableAgent?.available) return t("task:workflowMoveProfileUnavailable");
  return capabilityReason(
    t,
    profileCapabilityStatus(profile, availableAgent),
    profile.capability_error ?? availableAgent.model_config.error,
  );
}

function profileOptionsForMove(
  t: ReturnType<typeof useTranslation>["t"],
  profiles: AgentProfileOption[],
  availableAgents: AvailableAgent[],
  capabilitiesLoaded: boolean,
): ComboboxOption[] {
  const targetDefault: ComboboxOption = {
    value: "",
    label: t("task:workflowMoveTargetDefault"),
  };
  return [
    targetDefault,
    ...profiles.map((profile): ComboboxOption => {
      const availableAgent = availableAgents.find((agent) => agent.name === profile.agent_name);
      const disabledReason = profileDisabledReason(t, profile, availableAgent, capabilitiesLoaded);
      const disabled = Boolean(disabledReason);
      return {
        value: profile.id,
        label: profile.label,
        disabled,
        disabledReason,
        keywords: [profile.label, profile.id, profile.agent_name],
        renderLabel: () => (
          <span
            className="truncate"
            title={profile.label}
            data-testid={`workflow-move-profile-option-${profile.id}`}
          >
            {profile.label}
          </span>
        ),
      };
    }),
  ];
}

/**
 * Draft state plus the profile capability list shared by every move-options
 * surface (stepper disclosure, proceed popover, dialog, drawer). The model
 * override stays in the draft/payload type for API compatibility, but this
 * PR's web UI never renders a model control.
 */
export function useWorkflowMoveOptionsForm() {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<WorkflowMoveOptionsDraft>(EMPTY_DRAFT);
  const agentProfiles = useAppStore((state) => state.agentProfiles?.items) ?? [];
  const availableAgentsState =
    useAppStore((state) => state.availableAgents) ??
    ({ items: [], loading: false, loaded: false, tools: [] } as const);

  // Keep unavailable profiles visible as disabled options with an explanation;
  // hiding them would make a configured target look like an arbitrary model
  // validation failure and would prevent users from understanding why it is
  // unavailable for this one-time entry.
  const patchDraft = useCallback((patch: Partial<WorkflowMoveOptionsDraft>) => {
    setDraft((current) => ({ ...current, ...patch }));
  }, []);

  const profileOptions = useMemo(
    () =>
      profileOptionsForMove(
        t,
        agentProfiles,
        availableAgentsState.items,
        availableAgentsState.loaded,
      ),
    [availableAgentsState.items, availableAgentsState.loaded, agentProfiles, t],
  );

  return { draft, patchDraft, profileOptions };
}

type WorkflowMoveOptionsFieldsProps = {
  draft: WorkflowMoveOptionsDraft;
  onDraftChange: (patch: Partial<WorkflowMoveOptionsDraft>) => void;
  profileOptions: ComboboxOption[];
  isTouchSurface: boolean;
  instructionsRows?: number;
};

/** Profile, reset-context, and one-time instructions fields (no model control). */
export function WorkflowMoveOptionsFields({
  draft,
  onDraftChange,
  profileOptions,
  isTouchSurface,
  instructionsRows = 4,
}: WorkflowMoveOptionsFieldsProps) {
  const { t } = useTranslation();
  const touchTargetClass = isTouchSurface ? "min-h-11" : "h-7";
  return (
    <div className="grid gap-3.5 text-sm">
      <div className="grid gap-1.5">
        <span>{t("task:workflowMoveAgentProfile")}</span>
        <Combobox
          options={profileOptions}
          value={draft.agentProfileId}
          onValueChange={(agentProfileId) => onDraftChange({ agentProfileId })}
          ariaLabel={t("task:workflowMoveAgentProfile")}
          placeholder={t("task:workflowMoveTargetDefault")}
          searchPlaceholder={t("task:workflowMoveAgentProfilePlaceholder")}
          emptyMessage={t("task:workflowMoveProfileUnavailable")}
          dropdownLabel={t("task:workflowMoveAgentProfile")}
          testId="workflow-move-agent-profile"
          triggerClassName={cn(touchTargetClass, "border border-input bg-background px-3")}
        />
      </div>
      <label className={cn("flex items-center gap-3 text-sm", touchTargetClass)}>
        <Checkbox
          checked={draft.resetContext}
          onCheckedChange={(checked) => onDraftChange({ resetContext: checked === true })}
          data-testid="workflow-move-reset-context"
        />
        <span>{t("task:workflowMoveResetContext")}</span>
      </label>
      <label className="grid gap-1.5 text-sm">
        <span>{t("task:workflowMoveInstructions")}</span>
        <Textarea
          value={draft.instructions}
          onChange={(event) => onDraftChange({ instructions: event.target.value })}
          placeholder={t("task:workflowMoveInstructionsPlaceholder")}
          rows={instructionsRows}
          data-testid="workflow-move-instructions"
        />
      </label>
    </div>
  );
}

/** Result contract for move-options submits: `false` keeps the form open. */
export type WorkflowMoveOptionsSubmit = (
  options: WorkflowMoveEntryOptions | undefined,
) => boolean | void | Promise<boolean | void>;

type WorkflowMoveOptionsFormProps = {
  isMoving: boolean;
  isTouchSurface: boolean;
  instructionsRows?: number;
  onSubmit: WorkflowMoveOptionsSubmit;
  onCancel?: () => void;
};

/**
 * Complete self-contained options form: fields plus the Move submit action.
 * A submit that resolves to `false` (failed move) keeps the draft and the
 * form open so nothing the user typed is lost.
 */
export function WorkflowMoveOptionsForm({
  isMoving,
  isTouchSurface,
  instructionsRows,
  onSubmit,
  onCancel,
}: WorkflowMoveOptionsFormProps) {
  const { t } = useTranslation();
  const { draft, patchDraft, profileOptions } = useWorkflowMoveOptionsForm();
  const [submitting, setSubmitting] = useState(false);
  const busy = isMoving || submitting;

  const submit = async () => {
    if (busy) return;
    setSubmitting(true);
    try {
      await onSubmit(workflowMoveOptionsPayload(draft));
    } finally {
      setSubmitting(false);
    }
  };

  const touchTargetClass = isTouchSurface ? "min-h-11" : "h-7";
  return (
    <div className="grid gap-4">
      <WorkflowMoveOptionsFields
        draft={draft}
        onDraftChange={patchDraft}
        profileOptions={profileOptions}
        isTouchSurface={isTouchSurface}
        instructionsRows={instructionsRows}
      />
      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={touchTargetClass}
            onClick={onCancel}
          >
            {t("common:cancel")}
          </Button>
        )}
        <Button
          type="button"
          size="sm"
          className={touchTargetClass}
          disabled={busy}
          onClick={submit}
          data-testid="workflow-move-submit"
        >
          {busy ? t("task:moving") : t("task:workflowMoveApply")}
        </Button>
      </div>
    </div>
  );
}

type WorkflowMoveOptionsProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targetStepName: string;
  isMoving?: boolean;
  onSubmit: WorkflowMoveOptionsSubmit;
};

/** Dialog (fine pointer) / Drawer (touch) wrapper around the shared form. */
export function WorkflowMoveOptions({
  open,
  onOpenChange,
  targetStepName,
  isMoving = false,
  onSubmit,
}: WorkflowMoveOptionsProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const form = (
    <WorkflowMoveOptionsForm
      isMoving={isMoving}
      isTouchSurface={usesTouchDrawer}
      onSubmit={onSubmit}
      onCancel={() => onOpenChange(false)}
    />
  );

  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="!top-0 !bottom-auto !mt-0 !h-dvh !max-h-dvh min-h-0 overflow-hidden pb-[env(safe-area-inset-bottom,0px)]">
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>
              {t("task:workflowMoveOptionsTitle", { step: targetStepName })}
            </DrawerTitle>
            <DrawerDescription>{t("task:workflowMoveOptionsDescription")}</DrawerDescription>
          </DrawerHeader>
          <div
            className="min-h-0 flex-1 overflow-y-auto px-4"
            data-vaul-no-drag
            data-testid="workflow-move-options"
          >
            {form}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("task:workflowMoveOptionsTitle", { step: targetStepName })}</DialogTitle>
          <DialogDescription>{t("task:workflowMoveOptionsDescription")}</DialogDescription>
        </DialogHeader>
        <div data-testid="workflow-move-options">{form}</div>
      </DialogContent>
    </Dialog>
  );
}
