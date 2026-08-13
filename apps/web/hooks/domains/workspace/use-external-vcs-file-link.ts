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
    return `${parsed.host}${parsed.pathname.replace(/\/+$/, "")}`.toLowerCase();
  } catch {
    return "";
  }
}

function repositoryFromIdentity(
  identity: GitRemoteRefObservation["identity"],
): ExternalVcsRepository | null {
  if (!identity?.repository?.host || !identity.ref) return null;
  const repository = identity.repository;
  const parts = repository.repository_path?.split("/") ?? [];
  if (!repository.repository_path && !repository.provider_repository_id) return null;
  const provider = repository.provider === "azure_repos" ? "azure_devops" : repository.provider ?? "";
  const isAzure = provider === "azure_devops";
  return {
    provider,
    provider_host: repository.host,
    provider_owner: isAzure ? parts.at(-2) ?? "" : parts.slice(0, -1).join("/"),
    provider_name: parts.at(-1) ?? "",
    provider_organization: isAzure ? parts[0] : undefined,
    provider_project: isAzure ? parts.at(-2) : undefined,
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

function identityFieldsPresent(value: {
  host?: string;
  owner?: string;
  repo?: string;
  id?: number;
  node?: string;
}): boolean {
  return value.host !== undefined || value.owner !== undefined || value.repo !== undefined || value.id !== undefined || value.node !== undefined;
}

function githubSource(pr: TaskPR): ExternalVcsRepositoryRef | null {
  const hasFields = identityFieldsPresent({ host: pr.head_host, owner: pr.head_owner, repo: pr.head_repo, id: pr.head_repo_id, node: pr.head_repo_node_id });
  if (hasFields && (!pr.head_host || !pr.head_owner || !pr.head_repo)) return null;
  if (!hasFields) return null;
  return {
    repository: {
      provider: "github",
      provider_host: pr.head_host,
      provider_owner: pr.head_owner,
      provider_name: pr.head_repo,
      provider_repository_id: pr.head_repo_id !== undefined ? String(pr.head_repo_id) : pr.head_repo_node_id,
    },
    ref: pr.head_branch,
  };
}

function githubBase(pr: TaskPR): ExternalVcsRepositoryRef | null {
  const hasFields = identityFieldsPresent({ host: pr.base_host, owner: pr.base_owner, repo: pr.base_repo, id: pr.base_repo_id });
  if (hasFields && (!pr.base_host || !pr.base_owner || !pr.base_repo)) return null;
  if (!hasFields) return null;
  return {
    repository: {
      provider: "github",
      provider_host: pr.base_host,
      provider_owner: pr.base_owner,
      provider_name: pr.base_repo,
      provider_repository_id: pr.base_repo_id !== undefined ? String(pr.base_repo_id) : undefined,
    },
    ref: pr.base_branch,
  };
}

function gitlabSource(mr: TaskMR): ExternalVcsRepositoryRef | null {
  const hasFields = mr.source_host !== undefined || mr.source_project_path !== undefined || mr.source_project_id !== undefined;
  if (hasFields && (!mr.source_host || !mr.source_project_path)) return null;
  if (!hasFields) return null;
  const parts = mr.source_project_path!.split("/");
  return {
    repository: {
      provider: "gitlab",
      provider_host: mr.source_host,
      provider_owner: parts.slice(0, -1).join("/"),
      provider_name: parts.at(-1) ?? "",
      provider_repository_id: mr.source_project_id !== undefined ? String(mr.source_project_id) : undefined,
    },
    ref: mr.head_branch,
  };
}

function gitlabBase(mr: TaskMR): ExternalVcsRepositoryRef | null {
  const hasFields = mr.target_host !== undefined || mr.target_project_path !== undefined || mr.target_project_id !== undefined;
  if (hasFields && (!mr.target_host || !mr.target_project_path)) return null;
  if (!hasFields) return null;
  const parts = mr.target_project_path!.split("/");
  return {
    repository: {
      provider: "gitlab",
      provider_host: mr.target_host,
      provider_owner: parts.slice(0, -1).join("/"),
      provider_name: parts.at(-1) ?? "",
      provider_repository_id: mr.target_project_id !== undefined ? String(mr.target_project_id) : undefined,
    },
    ref: mr.base_branch,
  };
}

function azureSource(pr: AzureDevOpsTaskPullRequest): ExternalVcsRepositoryRef | null {
  const hasFields = pr.sourceOrganizationUrl !== undefined || pr.sourceProjectId !== undefined || pr.sourceProjectName !== undefined || pr.sourceRepositoryId !== undefined || pr.sourceRepositoryName !== undefined;
  if (hasFields && (!pr.sourceOrganizationUrl || !pr.sourceProjectName || !pr.sourceRepositoryName)) return null;
  if (!hasFields) return null;
  return {
    repository: {
      provider: "azure_devops",
      provider_host: pr.sourceOrganizationUrl,
      provider_project: pr.sourceProjectName,
      provider_name: pr.sourceRepositoryName,
      provider_repository_id: pr.sourceRepositoryId,
    },
    ref: pr.sourceBranch,
  };
}

function azureBase(pr: AzureDevOpsTaskPullRequest): ExternalVcsRepositoryRef | null {
  const hasFields = pr.targetOrganizationUrl !== undefined || pr.targetProjectId !== undefined || pr.targetProjectName !== undefined || pr.targetRepositoryId !== undefined || pr.targetRepositoryName !== undefined;
  if (hasFields && (!pr.targetOrganizationUrl || !pr.targetProjectName || !pr.targetRepositoryName)) return null;
  if (!hasFields) return null;
  return {
    repository: {
      provider: "azure_devops",
      provider_host: pr.targetOrganizationUrl,
      provider_project: pr.targetProjectName,
      provider_name: pr.targetRepositoryName,
      provider_repository_id: pr.targetRepositoryId,
    },
    ref: pr.targetBranch,
  };
}

function sameRepositoryIdentity(left: ExternalVcsRepository, right: ExternalVcsRepository): boolean {
  const leftProvider = left.provider.toLowerCase() === "azure_repos" ? "azure_devops" : left.provider.toLowerCase();
  const rightProvider = right.provider.toLowerCase() === "azure_repos" ? "azure_devops" : right.provider.toLowerCase();
  if (leftProvider !== rightProvider) return false;
  if (providerHost(left.provider_host) !== providerHost(right.provider_host)) return false;
  if (left.provider_repository_id && right.provider_repository_id) {
    return left.provider_repository_id === right.provider_repository_id;
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

  const statusEntry = input.repositoryName
    ? snapshot.gitStatuses.find((entry) => entry.repository_name === input.repositoryName)?.status
    : snapshot.gitStatuses.length === 1
      ? snapshot.gitStatuses[0].status
      : null;
  const action = remoteRefFromObservation(statusEntry?.action_head);
  const comparisonTarget = statusEntry?.comparison?.target;
  const comparison =
    statusEntry?.comparison?.resolution_state === "resolved" && comparisonTarget?.ref
      ? repositoryFromIdentity({
          repository: comparisonTarget.repository,
          ref: comparisonTarget.ref,
        } as GitRemoteRefObservation["identity"])
        ? {
            repository: repositoryFromIdentity({
              repository: comparisonTarget.repository,
              ref: comparisonTarget.ref,
            } as GitRemoteRefObservation["identity"])!,
            ref: comparisonTarget.ref,
          }
        : null
      : null;

  const candidates =
    repository.provider === "github"
      ? snapshot.githubPRs.filter((pr) => githubPRMatches(pr, repository)).map((pr) => ({
          item: pr,
          source: githubSource(pr),
          base: githubBase(pr),
          sourceFields: identityFieldsPresent({ host: pr.head_host, owner: pr.head_owner, repo: pr.head_repo, id: pr.head_repo_id, node: pr.head_repo_node_id }),
          baseFields: identityFieldsPresent({ host: pr.base_host, owner: pr.base_owner, repo: pr.base_repo, id: pr.base_repo_id }),
        }))
      : repository.provider === "gitlab"
        ? snapshot.gitlabMRs.filter((mr) => gitlabMRMatches(mr, repository)).map((mr) => ({
            item: mr,
            source: gitlabSource(mr),
            base: gitlabBase(mr),
            sourceFields: mr.source_host !== undefined || mr.source_project_path !== undefined || mr.source_project_id !== undefined,
            baseFields: mr.target_host !== undefined || mr.target_project_path !== undefined || mr.target_project_id !== undefined,
          }))
        : snapshot.azurePRs.filter((pr) => pr.repositoryId === repository.id).map((pr) => ({
            item: pr,
            source: azureSource(pr),
            base: azureBase(pr),
            sourceFields: pr.sourceOrganizationUrl !== undefined || pr.sourceProjectId !== undefined || pr.sourceProjectName !== undefined || pr.sourceRepositoryId !== undefined || pr.sourceRepositoryName !== undefined,
            baseFields: pr.targetOrganizationUrl !== undefined || pr.targetProjectId !== undefined || pr.targetProjectName !== undefined || pr.targetRepositoryId !== undefined || pr.targetRepositoryName !== undefined,
          }));

  const identityAware = candidates.filter((candidate) => candidate.sourceFields);
  const exact = action ? identityAware.filter((candidate) => sameRefIdentity(candidate.source, action)) : [];
  if (identityAware.length > 0 && exact.length !== 1) return null;
  const legacyCandidates = candidates.filter((candidate) => {
    const branch = candidate.source?.ref ??
      ("head_branch" in candidate.item ? candidate.item.head_branch : candidate.item.sourceBranch);
    return !activeBranch || branch === activeBranch;
  });
  const linked =
    identityAware.length > 0
      ? exact[0]
      : legacyCandidates.length === 1
        ? legacyCandidates[0]
        : candidates.length === 1
          ? candidates[0]
          : null;
  if (candidates.length > 1 && !linked) return null;
  if (linked && (linked.sourceFields && !linked.source || linked.baseFields && !linked.base)) return null;
  if (
    linked?.source &&
    linked.base &&
    providerHost(linked.source.repository.provider_host) !== providerHost(linked.base.repository.provider_host)
  ) {
    return null;
  }

  const publishedBranch = input.publishedBranch || linked?.source?.ref || (!linked ? action?.ref : resolvePublishedBranch(input, repository, activeBranch, snapshot));
  const fileStatus = input.status?.trim().toLowerCase();
  if (["added", "untracked", "modified"].includes(fileStatus ?? "") && !publishedBranch) {
    return null;
  }
  const sourceRepository = linked?.source?.repository ?? (action && !linked ? action.repository : null);
  const linkedBase = linked?.base;
  const baseRepository = linkedBase?.repository ?? (!linked ? comparison?.repository : null);
  const allowLegacyComparisonFallback = !statusEntry || statusEntry.comparison === undefined;
  const baseBranch =
    input.baseBranch ||
    linkedBase?.ref ||
    (!linked ? comparison?.ref ?? (allowLegacyComparisonFallback ? taskRepository.base_branch : null) : taskRepository.base_branch);
  return resolveExternalVcsFileURL({
    repository,
    path: input.filePath,
    previousPath: input.previousPath,
    status: input.status,
    publishedBranch,
    sourceRepository,
    sourceBranch: publishedBranch,
    baseRepository,
    baseBranch,
    comparisonRepository: comparison?.repository,
    comparisonBranch: comparison?.ref,
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
