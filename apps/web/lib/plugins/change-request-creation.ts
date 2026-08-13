import type { PluginRepositoryProviderRegistration } from "./registry";
import type { PluginHostRepository } from "./types";
import type {
  GitStatusEntry,
  GitRemoteRefObservation,
} from "@/lib/state/slices/session-runtime/types";
import { t } from "@/lib/i18n";

type TaskRepositoryLink = { repository_id: string; position?: number };
type CreationTask = {
  id: string;
  workspaceId?: string;
  workspace_id?: string;
  repositoryId?: string | null;
  repositories?: readonly TaskRepositoryLink[];
};
type CreationRepository = Pick<PluginHostRepository, "id" | "name" | "provider"> &
  Partial<Pick<PluginHostRepository, "workspace_id">>;

export type ChangeRequestProviderTarget = {
  provider: PluginRepositoryProviderRegistration;
  workspaceId: string;
  taskId: string;
  repositoryId: string;
  repository: PluginHostRepository;
};

function hasCompleteRemoteIdentity(value: GitRemoteRefObservation["identity"]): boolean {
  return Boolean(
    value?.ref &&
    value.repository?.host &&
    (value.repository.repository_path || value.repository.provider_repository_id),
  );
}

/**
 * Checks the evidence required immediately before a registered provider
 * creates a change request. The action head is the exact source and the
 * delivered comparison target is the exact base; either role being absent,
 * stale, or unresolved must fail closed.
 */
export function hasCompleteChangeRequestRemoteRoleEvidence(
  status:
    | Pick<GitStatusEntry, "remote_roles_generation" | "action_head" | "comparison">
    | undefined,
): boolean {
  if (!status?.remote_roles_generation) return false;
  const actionHead = status.action_head;
  const comparison = status.comparison;
  if (!actionHead || !hasCompleteRemoteIdentity(actionHead.identity)) return false;
  if (actionHead.observation_state !== "present" && actionHead.observation_state !== "absent") {
    return false;
  }
  if (actionHead.observation_state === "present" && !actionHead.remote_head_commit) return false;
  if (
    !comparison ||
    comparison.resolution_state !== "resolved" ||
    !comparison.context_generation ||
    !hasCompleteRemoteIdentity(comparison.target)
  ) {
    return false;
  }
  return true;
}

function taskRepositoryLinks(task: CreationTask): readonly TaskRepositoryLink[] {
  if (task.repositories?.length) {
    return [...task.repositories].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
  }
  if (task.repositoryId) return [{ repository_id: task.repositoryId, position: 0 }];
  return [];
}

export function resolveChangeRequestProviderTarget({
  task,
  repositories,
  repositoryScope,
  getProvider,
}: {
  task: CreationTask | undefined;
  repositories: readonly CreationRepository[];
  repositoryScope?: string;
  getProvider(id: string): PluginRepositoryProviderRegistration | undefined;
}): ChangeRequestProviderTarget | null {
  if (!task) return null;
  const workspaceId = task.workspaceId ?? task.workspace_id;
  if (!workspaceId) return null;
  const links = taskRepositoryLinks(task);
  const linkedIds = new Set(links.map((link) => link.repository_id));
  const linked = repositories.filter(
    (repository) =>
      linkedIds.has(repository.id) &&
      (!repository.workspace_id || repository.workspace_id === workspaceId),
  );
  const matches =
    repositoryScope === undefined || repositoryScope === ""
      ? linked.filter((repository) => repository.id === links[0]?.repository_id)
      : linked.filter((repository) => repository.name === repositoryScope);
  if (matches.length !== 1) return null;
  const repository = matches[0];
  const provider = getProvider(repository.provider);
  if (!provider?.createChangeRequest) return null;
  return {
    provider,
    workspaceId,
    taskId: task.id,
    repositoryId: repository.id,
    repository: {
      ...repository,
      workspace_id: repository.workspace_id ?? workspaceId,
    },
  };
}

type PushResult = { success: boolean; output?: string; error?: string };
type NativeCreateResult = {
  success: boolean;
  branch_pushed: boolean;
  pr_url?: string;
  provider?: string;
  output?: string;
  error?: string;
  linked?: boolean;
  association_error?: string;
};

async function remoteRolesAuthorized(
  revalidateRemoteRoles: (() => boolean | Promise<boolean>) | undefined,
): Promise<boolean> {
  if (!revalidateRemoteRoles) return true;
  try {
    return (await revalidateRemoteRoles()) === true;
  } catch {
    return false;
  }
}

type PreparedBranch = {
  success: boolean;
  branchPushed: boolean;
  output: string;
  error?: string;
};

async function prepareChangeRequestBranch({
  branchAlreadyPushed,
  push,
  repositoryScope,
  revalidateRemoteRoles,
}: {
  branchAlreadyPushed: boolean;
  push(options: { setUpstream: boolean }, repositoryScope?: string): Promise<PushResult>;
  repositoryScope?: string;
  revalidateRemoteRoles?: () => boolean | Promise<boolean>;
}): Promise<PreparedBranch> {
  if (!branchAlreadyPushed && !(await remoteRolesAuthorized(revalidateRemoteRoles))) {
    return {
      success: false,
      branchPushed: false,
      output: "",
      error: "remote_role_expectation_unavailable",
    };
  }
  if (branchAlreadyPushed) {
    return { success: true, branchPushed: true, output: "" };
  }
  const pushed = await push({ setUpstream: true }, repositoryScope);
  const output = pushed.output ?? "";
  if (!pushed.success) {
    return {
      success: false,
      branchPushed: false,
      output,
      ...(pushed.error ? { error: pushed.error } : {}),
    };
  }
  return { success: true, branchPushed: true, output };
}

export async function createChangeRequestWithProvider({
  target,
  push,
  repositoryScope,
  title,
  body,
  baseBranch,
  draft,
  branchAlreadyPushed,
  sessionId,
  signal,
  revalidateRemoteRoles,
}: {
  target: ChangeRequestProviderTarget;
  push(options: { setUpstream: boolean }, repositoryScope?: string): Promise<PushResult>;
  repositoryScope?: string;
  title: string;
  body: string;
  baseBranch?: string;
  draft: boolean;
  branchAlreadyPushed: boolean;
  sessionId: string;
  signal: AbortSignal;
  /** Rechecks current role generation/source/base immediately before creation. */
  revalidateRemoteRoles?: () => boolean | Promise<boolean>;
}): Promise<NativeCreateResult> {
  const prepared = await prepareChangeRequestBranch({
    branchAlreadyPushed,
    push,
    repositoryScope,
    revalidateRemoteRoles,
  });
  if (!prepared.success) {
    return {
      success: false,
      branch_pushed: prepared.branchPushed,
      output: prepared.output,
      ...(prepared.error ? { error: prepared.error } : {}),
    };
  }
  if (!(await remoteRolesAuthorized(revalidateRemoteRoles))) {
    return {
      success: false,
      branch_pushed: prepared.branchPushed,
      output: prepared.output,
      error: "remote_role_expectation_unavailable",
    };
  }
  try {
    const created = await target.provider.createChangeRequest!({
      workspaceId: target.workspaceId,
      taskId: target.taskId,
      sessionId,
      repositoryId: target.repositoryId,
      repository: target.repository,
      title,
      body,
      ...(baseBranch ? { baseBranch } : {}),
      draft: target.provider.supportsDraft === false ? false : draft,
      signal,
    });
    return {
      success: true,
      branch_pushed: true,
      pr_url: created.url,
      provider: created.provider ?? target.provider.id,
      output: created.output ?? prepared.output,
      ...(created.linked === undefined ? {} : { linked: created.linked }),
      ...(created.associationError ? { association_error: created.associationError } : {}),
    };
  } catch (error) {
    return {
      success: false,
      branch_pushed: true,
      output: prepared.output,
      error: error instanceof Error ? error.message : t("integrations:changeRequestCreationFailed"),
    };
  }
}
