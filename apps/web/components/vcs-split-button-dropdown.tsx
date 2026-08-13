"use client";

import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCloudDownload,
  IconCloudUpload,
  IconGitCherryPick,
  IconGitMerge,
  IconGitPullRequest,
} from "@tabler/icons-react";
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import { AHEAD_MARK, BEHIND_MARK, DEFAULT_BASE_BRANCH } from "./vcs-constants";
import { RemoteContributionActionItems } from "@/components/task/remote-contribution-action-items";

function StandardPushDropdownItems({
  disabled,
  hasUpstream,
  aheadCount,
  pushDisabled,
  onPush,
  disabledTitle,
}: {
  disabled: boolean;
  hasUpstream: boolean;
  aheadCount: number;
  pushDisabled: boolean;
  onPush: (force: boolean) => void;
  disabledTitle: string;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        className="cursor-pointer gap-3"
        disabled={disabled || pushDisabled}
        title={pushDisabled ? disabledTitle : undefined}
      >
        <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:push")}</span>
        {hasUpstream && aheadCount > 0 && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
            {AHEAD_MARK}
            {aheadCount}
          </span>
        )}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent>
        <DropdownMenuItem
          className="cursor-pointer gap-3"
          onClick={() => onPush(false)}
          disabled={disabled || pushDisabled}
          title={pushDisabled ? disabledTitle : undefined}
        >
          <IconCloudUpload className="h-4 w-4 text-muted-foreground" />
          <span>{t("integrations:push")}</span>
        </DropdownMenuItem>
        <DropdownMenuItem
          className="cursor-pointer gap-3"
          onClick={() => onPush(true)}
          disabled={disabled || pushDisabled}
          title={pushDisabled ? disabledTitle : undefined}
        >
          <IconAlertTriangle className="h-4 w-4 text-muted-foreground" />
          <span>{t("integrations:forcePush")}</span>
        </DropdownMenuItem>
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

function ContributionDropdownItems({
  disabled,
  replaceDisabled,
  useDisabled,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
  prNumber,
}: {
  disabled: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  onReplaceContribution: () => void;
  onUseContribution: () => void;
  onViewPRVersion: () => void;
  prNumber?: number;
}) {
  return (
    <>
      <DropdownMenuSeparator />
      <RemoteContributionActionItems
        disabled={disabled}
        replaceDisabled={replaceDisabled}
        useDisabled={useDisabled}
        onReplaceContribution={onReplaceContribution}
        onUseContribution={onUseContribution}
        onViewPRVersion={onViewPRVersion}
        prNumber={prNumber}
      />
    </>
  );
}

function CreatePRDropdownItem({
  disabled,
  canCreatePR,
  onPR,
}: Pick<VcsDropdownItemsProps, "disabled" | "canCreatePR" | "onPR">) {
  const { t } = useTranslation();
  return (
    <DropdownMenuItem
      className="cursor-pointer gap-3"
      onClick={onPR}
      disabled={disabled || !canCreatePR}
    >
      <IconGitPullRequest className="h-4 w-4 text-muted-foreground" />
      <span className="flex-1">{t("integrations:createPr")}</span>
    </DropdownMenuItem>
  );
}

function PullDropdownItem({
  disabled,
  pullDisabled,
  pullDisabledReason,
  hasUpstream,
  behindCount,
  onPull,
}: Pick<
  VcsDropdownItemsProps,
  "disabled" | "pullDisabled" | "pullDisabledReason" | "hasUpstream" | "behindCount" | "onPull"
>) {
  const { t } = useTranslation();
  const disabledTitle = pullDisabledReason ?? t("task:divergedActionsUnavailable");
  return (
    <DropdownMenuItem
      className="cursor-pointer gap-3"
      onClick={onPull}
      disabled={disabled || pullDisabled}
      title={pullDisabled ? disabledTitle : undefined}
    >
      <IconCloudDownload className="h-4 w-4 text-muted-foreground" />
      <span className="flex-1">{t("integrations:pull")}</span>
      {hasUpstream && behindCount > 0 && (
        <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
          {BEHIND_MARK}
          {behindCount}
        </span>
      )}
    </DropdownMenuItem>
  );
}

function ComparisonDropdownItems({
  disabled,
  comparisonEvidenceAvailable,
  baseBranch,
  onRebase,
  onMerge,
}: Pick<
  VcsDropdownItemsProps,
  "disabled" | "comparisonEvidenceAvailable" | "baseBranch" | "onRebase" | "onMerge"
>) {
  const { t } = useTranslation();
  const comparisonDisabled = disabled || !comparisonEvidenceAvailable;
  const branch = baseBranch || DEFAULT_BASE_BRANCH;
  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuItem
        className="cursor-pointer gap-3"
        onClick={onRebase}
        disabled={comparisonDisabled}
      >
        <IconGitCherryPick className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:rebase")}</span>
        <span className="text-xs text-muted-foreground">
          {t("integrations:ontoBranch", { branch })}
        </span>
      </DropdownMenuItem>
      <DropdownMenuItem
        className="cursor-pointer gap-3"
        onClick={onMerge}
        disabled={comparisonDisabled}
      >
        <IconGitMerge className="h-4 w-4 text-muted-foreground" />
        <span className="flex-1">{t("integrations:merge")}</span>
        <span className="text-xs text-muted-foreground">
          {t("integrations:fromBranch", { branch })}
        </span>
      </DropdownMenuItem>
    </>
  );
}

export type VcsDropdownItemsProps = {
  disabled: boolean;
  canCreatePR?: boolean;
  comparisonEvidenceAvailable?: boolean;
  baseBranch?: string;
  hasUpstream: boolean;
  behindCount: number;
  aheadCount: number;
  pushDisabled: boolean;
  pullDisabled: boolean;
  pushDisabledReason?: string;
  pullDisabledReason?: string;
  showContributionResolution: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  onPR: () => void;
  onPull: () => void;
  onPush: (force: boolean) => void;
  onReplaceContribution: () => void;
  onUseContribution: () => void;
  onViewPRVersion: () => void;
  prNumber?: number;
  onRebase: () => void;
  onMerge: () => void;
};

export function VcsDropdownItems({
  disabled,
  canCreatePR = false,
  comparisonEvidenceAvailable = false,
  baseBranch,
  hasUpstream,
  behindCount,
  aheadCount,
  pushDisabled,
  pullDisabled,
  pushDisabledReason,
  pullDisabledReason,
  showContributionResolution,
  replaceDisabled,
  useDisabled,
  onPR,
  onPull,
  onPush,
  onReplaceContribution,
  onUseContribution,
  onViewPRVersion,
  prNumber,
  onRebase,
  onMerge,
}: VcsDropdownItemsProps) {
  const { t } = useTranslation();
  const pushDisabledTitle = pushDisabledReason ?? t("task:divergedActionsUnavailable");
  return (
    <DropdownMenuContent align="end" className="w-56">
      <CreatePRDropdownItem disabled={disabled} canCreatePR={canCreatePR} onPR={onPR} />
      <DropdownMenuSeparator />
      <PullDropdownItem
        disabled={disabled}
        pullDisabled={pullDisabled}
        pullDisabledReason={pullDisabledReason}
        hasUpstream={hasUpstream}
        behindCount={behindCount}
        onPull={onPull}
      />
      {!showContributionResolution && (
        <StandardPushDropdownItems
          disabled={disabled}
          hasUpstream={hasUpstream}
          aheadCount={aheadCount}
          pushDisabled={pushDisabled}
          onPush={onPush}
          disabledTitle={pushDisabledTitle}
        />
      )}
      {showContributionResolution && (
        <ContributionDropdownItems
          disabled={disabled}
          replaceDisabled={replaceDisabled}
          useDisabled={useDisabled}
          onReplaceContribution={onReplaceContribution}
          onUseContribution={onUseContribution}
          onViewPRVersion={onViewPRVersion}
          prNumber={prNumber}
        />
      )}
      <ComparisonDropdownItems
        disabled={disabled}
        comparisonEvidenceAvailable={comparisonEvidenceAvailable}
        baseBranch={baseBranch}
        onRebase={onRebase}
        onMerge={onMerge}
      />
    </DropdownMenuContent>
  );
}
