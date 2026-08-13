"use client";

import { useState, useCallback, useMemo } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { t } from "@/lib/i18n";

// GitOperationResult matches the backend response
export interface GitOperationResult {
  success: boolean;
  operation: string;
  output: string;
  error?: string;
  error_code?: string;
  conflict_files?: string[];
  recovery_branch?: string;
}

// PRCreateResult matches the backend PR creation response
export interface PRCreateResult {
  success: boolean;
  branch_pushed?: boolean;
  pr_url?: string;
  provider?: string;
  output?: string;
  error?: string;
  linked?: boolean;
  association_error?: string;
}

export interface GitMutationExpectation {
  expected_remote_roles_generation?: string;
  expected_target?: unknown;
  expected_observation_state?: string;
  expected_remote_head_commit?: string;
  expected_comparison_context_generation?: string;
  expected_source?: unknown;
  expected_base?: unknown;
}

export function getChangeRequestTerminology(provider?: string) {
  return provider?.toLowerCase() === "gitlab"
    ? { longName: "Merge Request", shortName: "MR" }
    : { longName: "Pull Request", shortName: "PR" };
}

export function resolveChangeRequestTerminology(
  provider: string | undefined,
  fallback: ReturnType<typeof getChangeRequestTerminology>,
) {
  return provider ? getChangeRequestTerminology(provider) : fallback;
}

export function useChangeRequestTerminology(sessionId?: string | null, repoName?: string) {
  const provider = useAppStore((state) => {
    const taskId = sessionId ? state.taskSessions.items[sessionId]?.task_id : undefined;
    const task = taskId
      ? state.kanban.tasks.find((candidate: { id: string }) => candidate.id === taskId)
      : undefined;
    if (!task) return undefined;
    const repositories = Object.values(state.repositories.itemsByWorkspaceId).flat();
    const linkedRepositoryIds = new Set(
      task.repositories?.map((repository) => repository.repository_id) ?? [],
    );
    if (repoName) {
      return repositories.find(
        (repository) => linkedRepositoryIds.has(repository.id) && repository.name === repoName,
      )?.provider;
    }
    const primaryRepositoryId = task.repositories?.[0]?.repository_id;
    return repositories.find((repository) => repository.id === primaryRepositoryId)?.provider;
  });
  return useMemo(() => getChangeRequestTerminology(provider), [provider]);
}

interface UseGitOperationsReturn {
  // Operation methods. The optional `repo` parameter is the multi-repo subpath
  // (e.g. "kandev"); pass empty/undefined for single-repo workspaces. Multi-repo
  // workspaces MUST scope each call to one repo — bulk callers fan out themselves.
  pull: (
    rebase?: boolean,
    repo?: string,
    expectation?: GitMutationExpectation,
  ) => Promise<GitOperationResult>;
  push: (
    options?: { force?: boolean; setUpstream?: boolean; expectation?: GitMutationExpectation },
    repo?: string,
  ) => Promise<GitOperationResult>;
  replaceRemoteContribution: (
    expectedRemoteHead: string,
    repo?: string,
  ) => Promise<GitOperationResult>;
  useRemoteContribution: (expectedRemoteHead: string, repo?: string) => Promise<GitOperationResult>;
  rebase: (
    baseBranch: string,
    repo?: string,
    expectation?: GitMutationExpectation,
  ) => Promise<GitOperationResult>;
  merge: (
    baseBranch: string,
    repo?: string,
    expectation?: GitMutationExpectation,
  ) => Promise<GitOperationResult>;
  abort: (operation: "merge" | "rebase", repo?: string) => Promise<GitOperationResult>;
  commit: (
    message: string,
    stageAll?: boolean,
    amend?: boolean,
    repo?: string,
  ) => Promise<GitOperationResult>;
  stage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  unstage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  discard: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  revertCommit: (commitSHA: string, repo?: string) => Promise<GitOperationResult>;
  renameBranch: (newName: string, repo?: string) => Promise<GitOperationResult>;
  reset: (commitSHA: string, mode: "soft" | "hard", repo?: string) => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
    repo?: string,
    expectation?: GitMutationExpectation,
  ) => Promise<PRCreateResult>;

  // State
  isLoading: boolean;
  loadingOperation: string | null;
  error: string | null;
  lastResult: GitOperationResult | null;
}

type ExecuteOperation = <T extends GitOperationResult>(
  action: string,
  payload: Record<string, unknown>,
) => Promise<T>;

type OperationExpectation = (
  operation: "pull" | "push" | "comparison" | "create_pr",
  repo?: string,
) => GitMutationExpectation | undefined;

function hasCompleteRemoteIdentity(value: unknown): boolean {
  if (!value || typeof value !== "object") return false;
  const identity = value as {
    ref?: unknown;
    repository?: { host?: unknown; repository_path?: unknown; provider_repository_id?: unknown };
  };
  return Boolean(
    typeof identity.ref === "string" &&
    identity.ref.length > 0 &&
    identity.repository?.host &&
    (identity.repository.repository_path || identity.repository.provider_repository_id),
  );
}

function hasMutationExpectation(
  operation: "pull" | "push" | "comparison" | "create_pr",
  expectation: GitMutationExpectation | undefined,
): boolean {
  if (!expectation?.expected_remote_roles_generation) return false;
  if (!hasCompleteRemoteIdentity(expectation.expected_target)) return false;
  if (operation === "comparison") {
    return Boolean(expectation.expected_comparison_context_generation);
  }
  if (
    expectation.expected_observation_state !== "present" &&
    expectation.expected_observation_state !== "absent"
  ) {
    return false;
  }
  if (
    expectation.expected_observation_state === "present" &&
    !expectation.expected_remote_head_commit
  ) {
    return false;
  }
  if (operation === "create_pr") {
    return (
      Boolean(expectation.expected_comparison_context_generation) &&
      hasCompleteRemoteIdentity(expectation.expected_source) &&
      hasCompleteRemoteIdentity(expectation.expected_base)
    );
  }
  return true;
}

function unavailableMutationResult<T extends GitOperationResult>(operation: string): T {
  return {
    success: false,
    operation,
    output: "",
    error_code: "remote_role_expectation_unavailable",
  } as T;
}

function unavailableCreatePRResult(): PRCreateResult {
  return {
    success: false,
    error: "remote_role_expectation_unavailable",
  };
}

function buildContributionCallbacks(executeOperation: ExecuteOperation) {
  const replaceRemoteContribution = async (expectedRemoteHead: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.replace_contribution", {
      expected_remote_head: expectedRemoteHead,
      ...repositoryScopePayload(repo),
    });
  const useRemoteContribution = async (expectedRemoteHead: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.use_contribution", {
      expected_remote_head: expectedRemoteHead,
      ...repositoryScopePayload(repo),
    });
  return { replaceRemoteContribution, useRemoteContribution };
}

/** Preserve an explicitly selected workspace-root scope (`repo === ""`). */
export function repositoryScopePayload(repo?: string): { repo?: string } {
  return repo === undefined ? {} : { repo };
}

export function buildGitOperationCallbacks(
  executeOperation: ExecuteOperation,
  getExpectation?: OperationExpectation,
) {
  const { replaceRemoteContribution, useRemoteContribution } =
    buildContributionCallbacks(executeOperation);
  const pull = async (rebase = false, repo?: string, expectation?: GitMutationExpectation) => {
    const resolvedExpectation = expectation ?? getExpectation?.("pull", repo);
    if (getExpectation && !hasMutationExpectation("pull", resolvedExpectation)) {
      return unavailableMutationResult<GitOperationResult>("pull");
    }
    return executeOperation<GitOperationResult>("worktree.pull", {
      rebase,
      ...(resolvedExpectation ?? {}),
      ...repositoryScopePayload(repo),
    });
  };

  const push = async (
    options?: { force?: boolean; setUpstream?: boolean; expectation?: GitMutationExpectation },
    repo?: string,
  ) => {
    const resolvedExpectation = options?.expectation ?? getExpectation?.("push", repo);
    if (getExpectation && !hasMutationExpectation("push", resolvedExpectation)) {
      return unavailableMutationResult<GitOperationResult>("push");
    }
    return executeOperation<GitOperationResult>("worktree.push", {
      force: options?.force ?? false,
      set_upstream: options?.setUpstream ?? false,
      ...(resolvedExpectation ?? {}),
      ...repositoryScopePayload(repo),
    });
  };

  const rebase = async (
    baseBranch: string,
    repo?: string,
    expectation?: GitMutationExpectation,
  ) => {
    const resolvedExpectation = expectation ?? getExpectation?.("comparison", repo);
    if (getExpectation && !hasMutationExpectation("comparison", resolvedExpectation)) {
      return unavailableMutationResult<GitOperationResult>("rebase");
    }
    return executeOperation<GitOperationResult>("worktree.rebase", {
      base_branch: baseBranch,
      ...(resolvedExpectation ?? {}),
      ...repositoryScopePayload(repo),
    });
  };

  const merge = async (baseBranch: string, repo?: string, expectation?: GitMutationExpectation) => {
    const resolvedExpectation = expectation ?? getExpectation?.("comparison", repo);
    if (getExpectation && !hasMutationExpectation("comparison", resolvedExpectation)) {
      return unavailableMutationResult<GitOperationResult>("merge");
    }
    return executeOperation<GitOperationResult>("worktree.merge", {
      base_branch: baseBranch,
      ...(resolvedExpectation ?? {}),
      ...repositoryScopePayload(repo),
    });
  };

  const abort = async (operation: "merge" | "rebase", repo?: string) =>
    executeOperation<GitOperationResult>("worktree.abort", {
      operation,
      ...repositoryScopePayload(repo),
    });

  const commit = async (message: string, stageAll = true, amend = false, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.commit", {
      message,
      stage_all: stageAll,
      amend,
      ...repositoryScopePayload(repo),
    });

  const stage = async (paths?: string[], repo?: string) =>
    executeOperation<GitOperationResult>("worktree.stage", {
      paths: paths ?? [],
      ...repositoryScopePayload(repo),
    });

  const unstage = async (paths?: string[], repo?: string) =>
    executeOperation<GitOperationResult>("worktree.unstage", {
      paths: paths ?? [],
      ...repositoryScopePayload(repo),
    });

  const discard = async (paths?: string[], repo?: string) =>
    executeOperation<GitOperationResult>("worktree.discard", {
      paths: paths ?? [],
      ...repositoryScopePayload(repo),
    });

  const revertCommit = async (commitSHA: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.revert_commit", {
      commit_sha: commitSHA,
      ...repositoryScopePayload(repo),
    });

  const renameBranch = async (newName: string, repo?: string) =>
    executeOperation<GitOperationResult>("worktree.rename_branch", {
      new_name: newName,
      ...repositoryScopePayload(repo),
    });

  const reset = async (commitSHA: string, mode: "soft" | "hard", repo?: string) =>
    executeOperation<GitOperationResult>("worktree.reset", {
      commit_sha: commitSHA,
      mode,
      ...repositoryScopePayload(repo),
    });

  const createPR = async (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
    repo?: string,
    expectation?: GitMutationExpectation,
  ): Promise<PRCreateResult> => {
    const resolvedExpectation = expectation ?? getExpectation?.("create_pr", repo);
    if (getExpectation && !hasMutationExpectation("create_pr", resolvedExpectation)) {
      return unavailableCreatePRResult();
    }
    return executeOperation<PRCreateResult & GitOperationResult>("worktree.create_pr", {
      title,
      body,
      base_branch: baseBranch ?? "",
      draft: draft ?? true,
      ...(resolvedExpectation ?? {}),
      ...repositoryScopePayload(repo),
    });
  };

  return {
    pull,
    push,
    replaceRemoteContribution,
    useRemoteContribution,
    rebase,
    merge,
    abort,
    commit,
    stage,
    unstage,
    discard,
    revertCommit,
    renameBranch,
    reset,
    createPR,
  };
}

export function useGitOperations(sessionId: string | null): UseGitOperationsReturn {
  const [isLoading, setIsLoading] = useState(false);
  const [loadingOperation, setLoadingOperation] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<GitOperationResult | null>(null);

  const executeOperation = useCallback(
    async <T extends GitOperationResult>(
      action: string,
      payload: Record<string, unknown>,
    ): Promise<T> => {
      if (!sessionId) throw new Error("No session ID provided");
      const client = getWebSocketClient();
      if (!client) throw new Error("WebSocket not connected");

      setIsLoading(true);
      setLoadingOperation(action.replace("worktree.", ""));
      setError(null);

      const timeout = action === "worktree.create_pr" ? 120000 : 60000;
      try {
        const result = await client.request<T>(
          action,
          { session_id: sessionId, ...payload },
          timeout,
        );
        setLastResult(result);
        if (!result.success && result.error) setError(result.error);
        return result;
      } catch (e) {
        const errorMessage = e instanceof Error ? e.message : t("task:gitOperationFailedGeneric");
        setError(errorMessage);
        throw e;
      } finally {
        setIsLoading(false);
        setLoadingOperation(null);
      }
    },
    [sessionId],
  );

  const statusByRepo = useAppStore((state) => {
    if (!sessionId) return undefined;
    const environmentID = state.environmentIdBySessionId[sessionId] ?? sessionId;
    return state.gitStatus.byEnvironmentRepo[environmentID];
  });
  const getExpectation = useCallback<OperationExpectation>(
    (operation, repo) => {
      const status: GitStatusEntry | undefined = statusByRepo?.[repo ?? ""];
      if (!status?.remote_roles_generation) return undefined;
      if (operation === "comparison" && status.comparison?.resolution_state !== "resolved") {
        return undefined;
      }
      const role =
        operation === "push" || operation === "create_pr"
          ? status.action_head
          : operation === "pull"
            ? status.tracking_upstream
            : status.comparison?.target
              ? {
                  identity: status.comparison.target,
                  observation_state: status.comparison.resolution_state,
                  remote_head_commit: undefined,
                }
              : undefined;
      if (!role?.identity) return undefined;
      const expectation: GitMutationExpectation = {
        expected_remote_roles_generation: status.remote_roles_generation,
        expected_target: role.identity,
        expected_observation_state: operation === "comparison" ? undefined : role.observation_state,
        expected_remote_head_commit:
          operation === "comparison" ? undefined : role.remote_head_commit,
        expected_comparison_context_generation: status.comparison?.context_generation,
      };
      if (operation === "create_pr") {
        expectation.expected_source = status.action_head?.identity;
        expectation.expected_base = status.comparison?.target;
      }
      return expectation;
    },
    [statusByRepo],
  );
  const ops = useMemo(
    () => buildGitOperationCallbacks(executeOperation, getExpectation),
    [executeOperation, getExpectation],
  );

  return {
    ...ops,
    isLoading,
    loadingOperation,
    error,
    lastResult,
  };
}
