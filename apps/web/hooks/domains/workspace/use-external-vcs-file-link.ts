"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Repository, Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import type { ExternalVcsFileURL } from "@/lib/utils/external-vcs-file-url";
import {
  resolveExternalVcsFileURL,
  type ExternalVcsRepository,
  type ExternalVcsRepositoryRef,
} from "@/lib/utils/external-vcs-file-url";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useWorkspaceMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useAzureDevOpsTaskPullRequests } from "@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests";
import { useSessionGitStatusByRepo } from "@/hooks/domains/session/use-session-git-status";
import { hasComparisonEvidence } from "@/hooks/domains/session/use-session-git-derived";
import { providerCandidates, type LinkCandidate } from "./use-external-vcs-file-link-candidates";
import type { GitRemoteRefObservation, GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

export type UseExternalVcsFileLinkInput = {
  filePath: string;
  previousPath?: string | null;
  status?: string | null;
  taskId?: string | null;
  sessionId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string | null;
  publishedBranch?: string | null;
  baseBranch?: string | null;
};

type TaskRepositoryLink = NonNullable<KanbanState["tasks"][number]["repositories"]>[number];

const EMPTY_TASK_REPOSITORIES: TaskRepositoryLink[] = [];
const EMPTY_GITHUB_PRS: TaskPR[] = [];
const EMPTY_GITLAB_MRS: TaskMR[] = [];
const EMPTY_AZURE_PRS: AzureDevOpsTaskPullRequest[] = [];

type LinkSnapshot = {
  repositories: Repository[];
  taskRepositories: TaskRepositoryLink[];
  sessionRepositoryId?: string;
  sessionWorktreeBranch?: string;
  sessionWorktrees: Worktree[];
  githubPRs: TaskPR[];
  gitlabMRs: TaskMR[];
  azurePRs: AzureDevOpsTaskPullRequest[];
  gitStatuses: Array<{ repository_name: string; status: GitStatusEntry }>;
};

function providerHost(value: string | undefined): string {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) return "";
  try {
    const parsed = new URL(trimmed.includes("://") ? trimmed : `https://${trimmed}`);
    return parsed.host.toLowerCase();
  } catch {
    return "";
  }
}

function repositoryHost(value: string | undefined): string {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) return "";
  try {
    const parsed = new URL(trimmed.includes("://") ? trimmed : `https://${trimmed}`);
    return `${parsed.host}${parsed.pathname.replace(/\/+$/, "")}`.toLowerCase();
  } catch {
    return "";
  }
}

function azureIdentityHost(host: string, organization: string): string {
  try {
    const parsed = new URL(host.includes("://") ? host : `https://${host}`);
    return `${parsed.origin}/${organization}`;
  } catch {
    return `https://dev.azure.com/${organization}`;
  }
}

function identityRepositoryParts(
  repository: NonNullable<GitRemoteRefObservation["identity"]>["repository"],
  provider: string,
): { parts: string[]; owner: string; organization?: string; project?: string } | null {
  const parts = repository.repository_path?.split("/") ?? [];
  if (!repository.repository_path && !repository.provider_repository_id) return null;
  const isAzure = provider === "azure_devops";
  return {
    parts,
    owner: isAzure ? parts.at(-2) ?? "" : parts.slice(0, -1).join("/"),
    organization: isAzure ? parts[0] : undefined,
    project: isAzure ? parts.at(-2) : undefined,
  };
}

function repositoryFromIdentity(
  identity: GitRemoteRefObservation["identity"],
): ExternalVcsRepository | null {
  if (!identity?.repository.host || !identity.ref) return null;
  const repository = identity.repository;
  const provider = repository.provider === "azure_repos" ? "azure_devops" : repository.provider ?? "";
  const parts = identityRepositoryParts(repository, provider);
  if (!parts) return null;
  const providerHostValue = parts.organization
    ? azureIdentityHost(repository.host!, parts.organization)
    : repository.host;
  return {
    provider,
    provider_host: providerHostValue,
    provider_owner: parts.owner,
    provider_name: parts.parts.at(-1) ?? "",
    provider_organization: parts.organization,
    provider_project: parts.project,
    provider_repository_id: repository.provider_repository_id,
  };
}

function remoteRefFromObservation(
  observation: GitRemoteRefObservation | null | undefined,
): ExternalVcsRepositoryRef | null {
  const repository = repositoryFromIdentity(observation?.identity);
  const ref = observation?.identity?.ref?.trim();
  return repository && ref ? { repository, ref } : null;
}

function sameRepositoryIdentity(left: ExternalVcsRepository, right: ExternalVcsRepository): boolean {
  const leftProvider = left.provider.toLowerCase() === "azure_repos" ? "azure_devops" : left.provider.toLowerCase();
  const rightProvider = right.provider.toLowerCase() === "azure_repos" ? "azure_devops" : right.provider.toLowerCase();
  if (leftProvider !== rightProvider) return false;
  if (repositoryHost(left.provider_host) !== repositoryHost(right.provider_host)) return false;
  if (left.provider_repository_id && right.provider_repository_id) {
    if (left.provider_repository_id !== right.provider_repository_id) return false;
  }
  return Boolean(
    left.provider_owner && right.provider_owner && left.provider_name && right.provider_name &&
      left.provider_owner.toLowerCase() === right.provider_owner.toLowerCase() &&
      left.provider_name.toLowerCase() === right.provider_name.toLowerCase(),
  );
}

function sameRefIdentity(left: ExternalVcsRepositoryRef | null, right: ExternalVcsRepositoryRef | null): boolean {
  return Boolean(left && right && left.ref === right.ref && sameRepositoryIdentity(left.repository, right.repository));
}

function statusForInput(
  input: UseExternalVcsFileLinkInput,
  statuses: LinkSnapshot["gitStatuses"],
): GitStatusEntry | null {
  if (input.repositoryName) {
    return statuses.find((entry) => entry.repository_name === input.repositoryName)?.status ?? null;
  }
  return statuses.length === 1 ? statuses[0].status : null;
}

function comparisonFromStatus(status: GitStatusEntry | null): ExternalVcsRepositoryRef | null {
  const target = status?.comparison?.target;
  if (!status || !hasComparisonEvidence(status) || !target?.ref || !status.comparison?.resolved_ref) {
    return null;
  }
  const repository = repositoryFromIdentity({
    repository: target.repository,
    ref: target.ref,
  } as GitRemoteRefObservation["identity"]);
  return repository ? { repository, ref: status.comparison.resolved_ref } : null;
}

function selectLinkedCandidate(
  candidates: LinkCandidate[],
  action: ExternalVcsRepositoryRef | null,
): LinkCandidate | null | undefined {
  const identityAware = candidates.filter((candidate) => candidate.sourceFields);
  if (identityAware.length > 0) {
    const exact = action
      ? identityAware.filter((candidate) => sameRefIdentity(candidate.source, action))
      : identityAware;
    return exact.length === 1 ? exact[0] : undefined;
  }
  return candidates.length > 0 ? undefined : null;
}

function linkedCandidateInvalid(candidate: LinkCandidate | null): boolean {
  return Boolean(candidate && ((candidate.sourceFields && !candidate.source) || (candidate.baseFields && !candidate.base)));
}

function chooseLinkedCandidate(
  candidates: LinkCandidate[],
  action: ExternalVcsRepositoryRef | null,
): LinkCandidate | null | false {
  const linked = selectLinkedCandidate(candidates, action);
  if (linked === undefined || (candidates.length > 1 && !linked)) return false;
  return linkedCandidateInvalid(linked) ? false : linked;
}

function linkedSidesCrossHosts(candidate: LinkCandidate | null | false): boolean {
  if (!candidate || !candidate.source || !candidate.base) return false;
  return providerHost(candidate.source.repository.provider_host) !== providerHost(candidate.base.repository.provider_host);
}

function resolvedBaseBranch(
  input: UseExternalVcsFileLinkInput,
  linked: LinkCandidate | null,
  comparison: ExternalVcsRepositoryRef | null,
  status: GitStatusEntry | null,
  taskRepository: TaskRepositoryLink,
): string | null {
  if (linked?.base?.ref) return linked.base.ref;
  if (!linked && comparison?.ref) return comparison.ref;
  if (!linked && input.baseBranch) return input.baseBranch;
  if (!linked && (!status || status.comparison === undefined)) return taskRepository.base_branch;
  return null;
}

type LinkSides = {
  publishedBranch: string | null;
  sourceRepository: ExternalVcsRepository | null;
  baseRepository: ExternalVcsRepository | null;
  baseBranch: string | null;
  comparisonRepository: ExternalVcsRepository | null;
  comparisonBranch: string | null;
};

type LinkContext = {
  status: GitStatusEntry | null;
  action: ExternalVcsRepositoryRef | null;
  comparison: ExternalVcsRepositoryRef | null;
  linked: LinkCandidate | null | false;
};

function resolveLinkContext(
  input: UseExternalVcsFileLinkInput,
  snapshot: LinkSnapshot,
  repository: Repository,
): LinkContext {
  const status = statusForInput(input, snapshot.gitStatuses);
  const action = remoteRefFromObservation(status?.action_head);
  const comparison = comparisonFromStatus(status);
  const linked = chooseLinkedCandidate(
    providerCandidates(repository, snapshot),
    action,
  );
  return { status, action, comparison, linked };
}

function linkSource(context: LinkContext): ExternalVcsRepositoryRef | null {
  const linked = linkedCandidate(context);
  if (linked) return linked.source;
  return context.linked === null ? context.action : null;
}

function linkedCandidate(context: LinkContext): LinkCandidate | null {
  if (context.linked === false || !context.linked) return null;
  return context.linked;
}

function linkPublishedBranch(
  context: LinkContext,
  input: UseExternalVcsFileLinkInput,
  repository: Repository,
  activeBranch: string | null,
  snapshot: LinkSnapshot,
): string | null {
  const linked = linkedCandidate(context);
  if (linked?.source?.ref) return linked.source.ref;
  if (context.action?.ref) return context.action.ref;
  if (context.linked) return null;
  return resolvePublishedBranch(input, repository, activeBranch, snapshot);
}

function sourceRequired(status: string | null | undefined): boolean {
  return status === "added" || status === "untracked" || status === "modified";
}

function sourceMissing(
  status: string | null | undefined,
  source: ExternalVcsRepositoryRef | null,
  publishedBranch: string | null,
): boolean {
  return sourceRequired(status) && (!source || !publishedBranch);
}

function buildLinkSides({
  context,
  input,
  repository,
  activeBranch,
  snapshot,
  taskRepository,
}: {
  context: LinkContext;
  input: UseExternalVcsFileLinkInput;
  repository: Repository;
  activeBranch: string | null;
  snapshot: LinkSnapshot;
  taskRepository: TaskRepositoryLink;
}): LinkSides | null {
  const source = linkSource(context);
  const publishedBranch = linkPublishedBranch(
    context,
    input,
    repository,
    activeBranch,
    snapshot,
  );
  const fileStatus = input.status?.trim().toLowerCase();
  if (sourceMissing(fileStatus, source, publishedBranch)) return null;
  const linkedBase = linkedCandidate(context)?.base;
  return {
    publishedBranch,
    sourceRepository: source?.repository ?? null,
    baseRepository:
      linkedBase?.repository ??
      (context.linked === null ? context.comparison?.repository ?? null : null),
    baseBranch: resolvedBaseBranch(
      input,
      linkedCandidate(context),
      context.comparison,
      context.status,
      taskRepository,
    ),
    comparisonRepository: context.comparison?.repository ?? null,
    comparisonBranch: context.comparison?.ref ?? null,
  };
}

function resolveLinkSides(
  input: UseExternalVcsFileLinkInput,
  snapshot: LinkSnapshot,
  repository: Repository,
  activeBranch: string | null,
  taskRepository: TaskRepositoryLink,
): LinkSides | null {
  const context = resolveLinkContext(input, snapshot, repository);
  if (context.linked === false || linkedSidesCrossHosts(context.linked)) return null;
  return buildLinkSides({ context, input, repository, activeBranch, snapshot, taskRepository });
}

function sanitizedRepositoryName(value: string): string {
  return value
    .replace(/[^A-Za-z0-9_.-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^[-.]+|[-.]+$/g, "");
}

function repositoryMatchesName(repository: Repository, requestedName: string): boolean {
  const requested = sanitizedRepositoryName(requestedName);
  return [repository.name, repository.provider_name].some(
    (name) => name === requestedName || sanitizedRepositoryName(name) === requested,
  );
}

function linkedRepositoryIds(taskRepositories: TaskRepositoryLink[]): Set<string> {
  return new Set(taskRepositories.map((link) => link.repository_id));
}

function linkedRepositoryById(
  repositoryId: string | undefined,
  snapshot: LinkSnapshot,
  linkedIds: Set<string>,
): Repository | null {
  return (
    snapshot.repositories.find(
      (repository) => repository.id === repositoryId && linkedIds.has(repository.id),
    ) ?? null
  );
}

function resolveNamedRepository(
  repositoryName: string,
  snapshot: LinkSnapshot,
  linkedIds: Set<string>,
): Repository | null {
  const namedWorktrees = snapshot.sessionWorktrees.filter(
    (worktree) => basename(worktree.path) === repositoryName,
  );
  if (namedWorktrees.length > 0) {
    return namedWorktrees.length === 1
      ? linkedRepositoryById(namedWorktrees[0].repositoryId, snapshot, linkedIds)
      : null;
  }
  const matches = snapshot.repositories.filter(
    (repository) =>
      linkedIds.has(repository.id) && repositoryMatchesName(repository, repositoryName),
  );
  return matches.length === 1 ? matches[0] : null;
}

function resolveRepository(
  input: UseExternalVcsFileLinkInput,
  snapshot: LinkSnapshot,
): Repository | null {
  const linkedIds = linkedRepositoryIds(snapshot.taskRepositories);
  if (input.repositoryId) {
    return linkedRepositoryById(input.repositoryId, snapshot, linkedIds);
  }
  if (input.repositoryName) {
    return resolveNamedRepository(input.repositoryName, snapshot, linkedIds);
  }

  const worktreeRepositoryIds = new Set(
    snapshot.sessionWorktrees.map((worktree) => worktree.repositoryId).filter(Boolean),
  );
  if (worktreeRepositoryIds.size === 1) {
    const id = Array.from(worktreeRepositoryIds)[0];
    return linkedRepositoryById(id, snapshot, linkedIds);
  }
  if (worktreeRepositoryIds.size === 0 && snapshot.sessionRepositoryId) {
    return linkedRepositoryById(snapshot.sessionRepositoryId, snapshot, linkedIds);
  }
  if (worktreeRepositoryIds.size === 0 && !snapshot.sessionRepositoryId && linkedIds.size === 1) {
    const id = Array.from(linkedIds)[0];
    return snapshot.repositories.find((repository) => repository.id === id) ?? null;
  }
  return null;
}

function basename(path: string | undefined): string {
  return (
    path
      ?.replace(/[\\/]+$/, "")
      .split(/[\\/]/)
      .pop() ?? ""
  );
}

function resolveActiveBranch(
  input: UseExternalVcsFileLinkInput,
  repository: Repository,
  snapshot: LinkSnapshot,
): string | null {
  const repositoryWorktrees = snapshot.sessionWorktrees.filter(
    (worktree) => worktree.repositoryId === repository.id,
  );
  if (input.repositoryName) {
    const named = repositoryWorktrees.filter(
      (worktree) => basename(worktree.path) === input.repositoryName,
    );
    if (named.length === 1) return named[0].branch ?? null;
  }
  if (repositoryWorktrees.length === 1) return repositoryWorktrees[0].branch ?? null;
  if (
    repositoryWorktrees.length === 0 &&
    snapshot.sessionRepositoryId === repository.id &&
    snapshot.sessionWorktreeBranch
  ) {
    return snapshot.sessionWorktreeBranch;
  }
  return null;
}

function resolveTaskRepositoryLink(
  repository: Repository,
  activeBranch: string | null,
  snapshot: LinkSnapshot,
): TaskRepositoryLink | null {
  const links = snapshot.taskRepositories.filter((link) => link.repository_id === repository.id);
  if (links.length === 1) return links[0];
  if (!activeBranch) return null;
  const matches = links.filter(
    (link) => (link.checkout_branch || link.base_branch) === activeBranch,
  );
  return matches.length === 1 ? matches[0] : null;
}

function normalizeOrigin(value: string | undefined): string {
  try {
    return new URL(value ?? "").origin.toLowerCase();
  } catch {
    return "";
  }
}

function githubPRMatches(pr: TaskPR, repository: Repository): boolean {
  if (pr.repository_id) return pr.repository_id === repository.id;
  return (
    repository.provider === "github" &&
    pr.owner === repository.provider_owner &&
    pr.repo === repository.provider_name
  );
}

function gitlabMRMatches(mr: TaskMR, repository: Repository): boolean {
  if (mr.repository_id) return mr.repository_id === repository.id;
  const mergeRequestOrigin = normalizeOrigin(mr.host);
  const repositoryOrigin = normalizeOrigin(repository.provider_host);
  return (
    repository.provider === "gitlab" &&
    Boolean(mergeRequestOrigin && repositoryOrigin) &&
    mergeRequestOrigin === repositoryOrigin &&
    mr.project_path === `${repository.provider_owner}/${repository.provider_name}`
  );
}

function publishedBranches(repository: Repository, snapshot: LinkSnapshot): string[] {
  if (repository.provider === "github") {
    return snapshot.githubPRs
      .filter((pr) => githubPRMatches(pr, repository))
      .map((pr) => pr.head_branch)
      .filter(Boolean);
  }
  if (repository.provider === "gitlab") {
    return snapshot.gitlabMRs
      .filter((mr) => gitlabMRMatches(mr, repository))
      .map((mr) => mr.head_branch)
      .filter(Boolean);
  }
  if (repository.provider === "azure_devops") {
    return snapshot.azurePRs
      .filter((pr) => pr.repositoryId === repository.id)
      .map((pr) => pr.sourceBranch)
      .filter(Boolean);
  }
  return [];
}

function resolvePublishedBranch(
  input: UseExternalVcsFileLinkInput,
  repository: Repository,
  activeBranch: string | null,
  snapshot: LinkSnapshot,
): string | null {
  // TaskPR supplies a branch and PR number, but not the head repository or a
  // fork discriminator. Preserve the published branch until that provenance is
  // available instead of guessing a base-repository pull ref.
  if (input.publishedBranch) return input.publishedBranch;
  const branches = Array.from(new Set(publishedBranches(repository, snapshot)));
  if (activeBranch && branches.includes(activeBranch)) return activeBranch;
  const repositoryLinkCount = snapshot.taskRepositories.filter(
    (link) => link.repository_id === repository.id,
  ).length;
  if (repositoryLinkCount > 1) return null;
  return branches.length === 1 ? branches[0] : null;
}

function resolveLink(
  input: UseExternalVcsFileLinkInput,
  snapshot: LinkSnapshot,
): ExternalVcsFileURL | null {
  const repository = resolveRepository(input, snapshot);
  if (!repository) return null;
  const activeBranch = resolveActiveBranch(input, repository, snapshot);
  const taskRepository = resolveTaskRepositoryLink(repository, activeBranch, snapshot);
  if (!taskRepository) return null;

  const sides = resolveLinkSides(input, snapshot, repository, activeBranch, taskRepository);
  if (!sides) return null;
  return resolveExternalVcsFileURL({
    repository,
    path: input.filePath,
    previousPath: input.previousPath,
    status: input.status,
    publishedBranch: sides.publishedBranch,
    sourceRepository: sides.sourceRepository,
    sourceBranch: sides.publishedBranch,
    baseRepository: sides.baseRepository,
    baseBranch: sides.baseBranch,
    comparisonRepository: sides.comparisonRepository,
    comparisonBranch: sides.comparisonBranch,
  });
}

export function useExternalVcsFileLink(
  input: UseExternalVcsFileLinkInput,
): ExternalVcsFileURL | null {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const session = useAppStore((state) =>
    input.sessionId ? state.taskSessions.items[input.sessionId] : undefined,
  );
  const resolvedTaskId = input.taskId ?? session?.task_id ?? activeTaskId;
  const taskRepositories = useAppStore(
    (state) =>
      state.kanban.tasks.find((task) => task.id === resolvedTaskId)?.repositories ??
      EMPTY_TASK_REPOSITORIES,
  );
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const worktrees = useAppStore((state) => state.worktrees.items);
  const sessionWorktreeIds = useAppStore((state) =>
    input.sessionId
      ? state.sessionWorktreesBySessionId.itemsBySessionId[input.sessionId]
      : undefined,
  );
  const githubPRs = useAppStore((state) =>
    resolvedTaskId
      ? (state.taskPRs.byTaskId[resolvedTaskId] ?? EMPTY_GITHUB_PRS)
      : EMPTY_GITHUB_PRS,
  );
  const taskMRsByWorkspace = useAppStore((state) => state.taskMRs.byWorkspaceId);
  const azurePRs = useAppStore((state) =>
    resolvedTaskId
      ? (state.azureDevOpsTaskPullRequests.byTaskId[resolvedTaskId] ?? EMPTY_AZURE_PRS)
      : EMPTY_AZURE_PRS,
  );
  const gitStatuses = useSessionGitStatusByRepo(input.sessionId ?? null);

  const apiWorktrees: Worktree[] = session
    ? (session.worktrees ?? []).map((worktree) => ({
        id: worktree.worktree_id || worktree.id,
        sessionId: session.id,
        repositoryId: worktree.repository_id,
        path: worktree.worktree_path,
        branch: worktree.worktree_branch,
      }))
    : [];
  const seen = new Set(apiWorktrees.map((worktree) => worktree.id));
  const liveWorktrees = (sessionWorktreeIds ?? [])
    .map((id) => worktrees[id])
    .filter((worktree): worktree is Worktree => Boolean(worktree) && !seen.has(worktree.id));
  const gitlabMRs = resolvedTaskId
    ? Object.values(taskMRsByWorkspace).flatMap(
        (taskMRs) => taskMRs[resolvedTaskId] ?? EMPTY_GITLAB_MRS,
      )
    : EMPTY_GITLAB_MRS;
  return resolveLink(input, {
    repositories: Object.values(repositoriesByWorkspace).flat(),
    taskRepositories,
    sessionRepositoryId: session?.repository_id,
    sessionWorktreeBranch: session?.worktree_branch,
    sessionWorktrees: apiWorktrees.concat(liveWorktrees),
    githubPRs,
    gitlabMRs,
    azurePRs,
    gitStatuses,
  });
}

export function useExternalVcsFileLinkHydration(
  task: Pick<Task, "id" | "workspace_id" | "repositories"> | null,
  repositories: Repository[],
): void {
  const providers = useMemo(() => {
    const repositoryIds = new Set(task?.repositories?.map((link) => link.repository_id) ?? []);
    return new Set(
      repositories
        .filter((repository) => repositoryIds.has(repository.id))
        .map((repository) => repository.provider),
    );
  }, [repositories, task?.repositories]);
  const taskId = task?.id ?? null;
  const workspaceId = task?.workspace_id ?? null;
  useTaskPR(providers.has("github") ? taskId : null);
  useWorkspaceMRs(providers.has("gitlab") ? workspaceId : null);
  useAzureDevOpsTaskPullRequests(
    providers.has("azure_devops") ? workspaceId : null,
    providers.has("azure_devops") ? taskId : null,
  );
}
