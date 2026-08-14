"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconArrowRight } from "@tabler/icons-react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useAppStore } from "@/components/state-provider";
import { Combobox, type ComboboxOption } from "@/components/combobox";
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

type WorkflowMoveOptionsProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targetStepName: string;
  isMoving?: boolean;
  onSubmit: (options: WorkflowMoveEntryOptions | undefined) => void;
};

type WorkflowMoveOptionsFieldsProps = {
  draft: WorkflowMoveOptionsDraft;
  onDraftChange: (patch: Partial<WorkflowMoveOptionsDraft>) => void;
  profileOptions: ComboboxOption[];
  modelOptions: ComboboxOption[];
};

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
      const status = profileCapabilityStatus(profile, availableAgent);
      const disabledReason = !capabilitiesLoaded
        ? t("task:workflowMoveCapabilitiesLoading")
        : profile.enabled === false
          ? t("task:workflowMoveProfileUnavailable")
          : !availableAgent?.available
            ? t("task:workflowMoveProfileUnavailable")
            : capabilityReason(t, status, profile.capability_error ?? availableAgent.model_config.error);
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

function modelOptionsForMove(
  t: ReturnType<typeof useTranslation>["t"],
  selectedProfileId: string,
  profiles: AgentProfileOption[],
  availableAgents: AvailableAgent[],
): ComboboxOption[] {
  const targetDefault: ComboboxOption = {
    value: "",
    label: t("task:workflowMoveTargetDefault"),
  };
  if (!selectedProfileId) return [targetDefault];

  const profile = profiles.find((candidate) => candidate.id === selectedProfileId);
  const availableAgent = availableAgents.find((agent) => agent.name === profile?.agent_name);
  if (!profile || !availableAgent || availableAgent.model_config.status !== "ok") {
    return [targetDefault];
  }

  const models = availableAgent.model_config.available_models.map((model) => ({
    value: model.id,
    label: model.name,
    description: model.id !== model.name ? model.id : undefined,
    keywords: [model.id, model.name, model.description ?? ""],
    renderLabel: () => (
      <span
        className="flex min-w-0 flex-col truncate"
        data-testid={`workflow-move-model-option-${model.id}`}
      >
        <span className="truncate">{model.name}</span>
        {model.id !== model.name && (
          <span className="truncate text-xs text-muted-foreground">{model.id}</span>
        )}
      </span>
    ),
  }));
  return [targetDefault, ...models];
}

function WorkflowMoveOptionsFields({
  draft,
  onDraftChange,
  profileOptions,
  modelOptions,
}: WorkflowMoveOptionsFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4 py-2">
      <label className="grid gap-1.5 text-sm">
        <span>{t("task:workflowMoveAgentProfile")}</span>
        <Combobox
          options={profileOptions}
          value={draft.agentProfileId}
          onValueChange={(agentProfileId) => onDraftChange({ agentProfileId, model: "" })}
          placeholder={t("task:workflowMoveTargetDefault")}
          searchPlaceholder={t("task:workflowMoveAgentProfilePlaceholder")}
          emptyMessage={t("task:workflowMoveProfileUnavailable")}
          dropdownLabel={t("task:workflowMoveAgentProfile")}
          testId="workflow-move-agent-profile"
          triggerClassName="min-h-11 border border-input bg-background px-3"
        />
      </label>
      <label className="grid gap-1.5 text-sm">
        <span>{t("task:workflowMoveModel")}</span>
        <Combobox
          options={modelOptions}
          value={draft.model}
          onValueChange={(model) => onDraftChange({ model })}
          placeholder={t("task:workflowMoveTargetDefault")}
          searchPlaceholder={t("task:workflowMoveModelPlaceholder")}
          emptyMessage={t("task:workflowMoveModelsUnavailable")}
          dropdownLabel={t("task:workflowMoveModel")}
          testId="workflow-move-model"
          triggerClassName="min-h-11 border border-input bg-background px-3"
          disabled={modelOptions.length <= 1 && !draft.agentProfileId}
        />
      </label>
      <label className="flex min-h-11 items-center gap-3 text-sm">
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
          rows={4}
          data-testid="workflow-move-instructions"
        />
      </label>
    </div>
  );
}

function WorkflowMoveOptionsActions({
  isMoving,
  onCancel,
  onSubmit,
}: {
  isMoving: boolean;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-2">
      <Button type="button" variant="outline" className="min-h-11" onClick={onCancel}>
        {t("common:cancel")}
      </Button>
      <Button
        type="button"
        className="min-h-11"
        disabled={isMoving}
        onClick={onSubmit}
        data-testid="workflow-move-submit"
      >
        {isMoving ? t("task:moving") : t("task:workflowMoveApply")}
      </Button>
    </div>
  );
}

export function WorkflowMoveOptions({
  open,
  onOpenChange,
  targetStepName,
  isMoving = false,
  onSubmit,
}: WorkflowMoveOptionsProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const agentProfiles = useAppStore((state) => state.agentProfiles?.items) ?? [];
  const availableAgentsState =
    useAppStore((state) => state.availableAgents) ??
    ({ items: [], loading: false, loaded: false, tools: [] } as const);
  const [draft, setDraft] = useState<WorkflowMoveOptionsDraft>(EMPTY_DRAFT);

  // Keep unavailable profiles visible as disabled options with an explanation;
  // hiding them would make a configured target look like an arbitrary model
  // validation failure and would prevent users from understanding why it is
  // unavailable for this one-time entry.
  const selectableProfiles = agentProfiles;
  const profileOptions = useMemo(
    () =>
      profileOptionsForMove(
        t,
        selectableProfiles,
        availableAgentsState.items,
        availableAgentsState.loaded,
      ),
    [availableAgentsState.items, availableAgentsState.loaded, selectableProfiles, t],
  );
  const modelOptions = useMemo(
    () => modelOptionsForMove(t, draft.agentProfileId, selectableProfiles, availableAgentsState.items),
    [availableAgentsState.items, draft.agentProfileId, selectableProfiles, t],
  );

  useEffect(() => {
    if (open) setDraft(EMPTY_DRAFT);
  }, [open]);

  useEffect(() => {
    if (!draft.model || modelOptions.some((option) => option.value === draft.model)) return;
    setDraft((current) => ({ ...current, model: "" }));
  }, [draft.model, modelOptions]);

  const content = (
    <WorkflowMoveOptionsFields
      draft={draft}
      onDraftChange={(patch) => setDraft((current) => ({ ...current, ...patch }))}
      profileOptions={profileOptions}
      modelOptions={modelOptions}
    />
  );

  const submit = () => onSubmit(workflowMoveOptionsPayload(draft));
  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="pb-[env(safe-area-inset-bottom,0px)]">
          <DrawerHeader className="text-left">
            <DrawerTitle>
              {t("task:workflowMoveOptionsTitle", { step: targetStepName })}
            </DrawerTitle>
            <DrawerDescription>{t("task:workflowMoveOptionsDescription")}</DrawerDescription>
          </DrawerHeader>
          <div
            className="max-h-[60vh] overflow-y-auto px-4"
            data-vaul-no-drag
            data-testid="workflow-move-options"
          >
            {content}
          </div>
          <DrawerFooter>
            <WorkflowMoveOptionsActions
              isMoving={isMoving}
              onCancel={() => onOpenChange(false)}
              onSubmit={submit}
            />
          </DrawerFooter>
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
        <div data-testid="workflow-move-options">{content}</div>
        <DialogFooter>
          <WorkflowMoveOptionsActions
            isMoving={isMoving}
            onCancel={() => onOpenChange(false)}
            onSubmit={submit}
          />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type WorkflowMoveProceedControlsProps = {
  nextStepName: string;
  onProceed: (options?: WorkflowMoveEntryOptions) => void;
  isMoving: boolean;
  directClassName: string;
  optionsClassName: string;
  directTestId: string;
  optionsTestId: string;
};

export function WorkflowMoveProceedControls({
  nextStepName,
  onProceed,
  isMoving,
  directClassName,
  optionsClassName,
  directTestId,
  optionsTestId,
}: WorkflowMoveProceedControlsProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={directClassName}
            onClick={() => onProceed()}
            disabled={isMoving}
            data-testid={directTestId}
          >
            {nextStepName}
            <IconArrowRight className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:moveTaskToTheNextWorkflow")}</TooltipContent>
      </Tooltip>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={optionsClassName}
        onClick={() => setOpen(true)}
        disabled={isMoving}
        data-testid={optionsTestId}
      >
        {t("task:moveWithOptions")}
      </Button>
      <WorkflowMoveOptions
        open={open}
        onOpenChange={setOpen}
        targetStepName={nextStepName}
        isMoving={isMoving}
        onSubmit={(options) => {
          setOpen(false);
          onProceed(options);
        }}
      />
    </>
  );
}
