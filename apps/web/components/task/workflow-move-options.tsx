"use client";

import { useEffect, useId, useMemo, useState } from "react";
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
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconArrowRight } from "@tabler/icons-react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useAppStore } from "@/components/state-provider";
import {
  isSelectableAgentProfile,
  type AgentProfileOption,
} from "@/lib/state/slices/settings/types";
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
  profileListId: string;
  modelListId: string;
  selectableProfiles: AgentProfileOption[];
  modelSuggestions: string[];
};

function WorkflowMoveOptionsFields({
  draft,
  onDraftChange,
  profileListId,
  modelListId,
  selectableProfiles,
  modelSuggestions,
}: WorkflowMoveOptionsFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4 py-2">
      <label className="flex min-h-11 items-center gap-3 text-sm">
        <Checkbox
          checked={draft.resetContext}
          onCheckedChange={(checked) => onDraftChange({ resetContext: checked === true })}
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
        />
      </label>
      <label className="grid gap-1.5 text-sm">
        <span>{t("task:workflowMoveAgentProfile")}</span>
        <Input
          value={draft.agentProfileId}
          onChange={(event) => onDraftChange({ agentProfileId: event.target.value })}
          placeholder={t("task:workflowMoveAgentProfilePlaceholder")}
          list={profileListId}
        />
        <datalist id={profileListId}>
          {selectableProfiles.map((profile) => (
            <option key={profile.id} value={profile.id} label={profile.label} />
          ))}
        </datalist>
      </label>
      <label className="grid gap-1.5 text-sm">
        <span>{t("task:workflowMoveModel")}</span>
        <Input
          value={draft.model}
          onChange={(event) => onDraftChange({ model: event.target.value })}
          placeholder={t("task:workflowMoveModelPlaceholder")}
          list={modelListId}
        />
        <datalist id={modelListId}>
          {modelSuggestions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
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
      <Button type="button" className="min-h-11" disabled={isMoving} onClick={onSubmit}>
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
  const profileListId = useId();
  const modelListId = useId();
  const agentProfiles = useAppStore((state) => state.agentProfiles?.items) ?? [];
  const availableAgents = useAppStore((state) => state.availableAgents?.items) ?? [];
  const [draft, setDraft] = useState<WorkflowMoveOptionsDraft>(EMPTY_DRAFT);

  const selectableProfiles = useMemo(
    () => agentProfiles.filter(isSelectableAgentProfile),
    [agentProfiles],
  );
  const modelSuggestions = useMemo(() => {
    const selectedProfile = selectableProfiles.find(
      (profile) => profile.id === draft.agentProfileId,
    );
    const availableAgent = availableAgents.find(
      (agent) => agent.name === selectedProfile?.agent_name,
    );
    return Array.from(
      new Set(
        [
          selectedProfile?.model,
          selectedProfile?.fallback_model,
          ...(availableAgent?.model_config.available_models.map((model) => model.id) ?? []),
        ].filter((model): model is string => Boolean(model)),
      ),
    );
  }, [availableAgents, draft.agentProfileId, selectableProfiles]);

  useEffect(() => {
    if (open) setDraft(EMPTY_DRAFT);
  }, [open]);

  const content = (
    <WorkflowMoveOptionsFields
      draft={draft}
      onDraftChange={(patch) => setDraft((current) => ({ ...current, ...patch }))}
      profileListId={profileListId}
      modelListId={modelListId}
      selectableProfiles={selectableProfiles}
      modelSuggestions={modelSuggestions}
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
          <div className="max-h-[60vh] overflow-y-auto px-4" data-vaul-no-drag>
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
        {content}
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
