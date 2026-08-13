import type { Repository } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import type { GitRemoteRefObservation } from "@/lib/state/slices/session-runtime/types";
import type {
  ExternalVcsRepository,
  ExternalVcsRepositoryRef,
} from "@/lib/utils/external-vcs-file-url";

export type LinkCandidate = {
  item: TaskPR | TaskMR | AzureDevOpsTaskPullRequest;
  source: ExternalVcsRepositoryRef | null;
  base: ExternalVcsRepositoryRef | null;
  sourceFields: boolean;
  baseFields: boolean;
};

export type ProviderCandidateSnapshot = {
  githubPRs: TaskPR[];
  gitlabMRs: TaskMR[];
  azurePRs: AzureDevOpsTaskPullRequest[];
};

function azureIdentityParts(
  repository: NonNullable<GitRemoteRefObservation["identity"]>["repository"],
  pathParts: string[],
): { parts: string[]; owner: string; organization: string; project: string } | null {
  let hostOrganization: string | undefined;
  try {
    const parsed = new URL(
      repository.host!.includes("://") ? repository.host! : `https://${repository.host}`,
    );
    hostOrganization = parsed.pathname.split("/").filter(Boolean)[0];
  } catch {
    return null;
  }
  const organization = hostOrganization ?? pathParts[0];
  let repositoryParts: string[];
  if (hostOrganization && pathParts[0] === hostOrganization) {
    repositoryParts = pathParts.slice(1);
  } else if (hostOrganization) {
    repositoryParts = pathParts;
  } else {
    repositoryParts = pathParts.slice(1);
  }
  if (!organization || repositoryParts.length !== 2) return null;
  return {
    parts: repositoryParts,
    owner: repositoryParts[0],
    organization,
    project: repositoryParts[0],
  };
}

function identityRepositoryParts(
  repository: NonNullable<GitRemoteRefObservation["identity"]>["repository"],
  provider: string,
): { parts: string[]; owner: string; organization?: string; project?: string } | null {
  const parts = repository.repository_path?.split("/").filter(Boolean) ?? [];
  if (!repository.repository_path) return null;
  if (provider === "azure_devops") return azureIdentityParts(repository, parts);
  return { parts, owner: parts.slice(0, -1).join("/") };
}

export function repositoryFromIdentity(
  identity: GitRemoteRefObservation["identity"],
): ExternalVcsRepository | null {
  if (!identity?.repository.host || !identity.ref) return null;
  const repository = identity.repository;
  const provider = repository.provider === "azure_repos" ? "azure_devops" : repository.provider ?? "";
  const parts = identityRepositoryParts(repository, provider);
  if (!parts) return null;
  return {
    provider,
    provider_host: parts.organization
      ? `https://dev.azure.com/${parts.organization}`
      : repository.host,
    provider_owner: parts.owner,
    provider_name: parts.parts.at(-1) ?? "",
    provider_organization: parts.organization,
    provider_project: parts.project,
    provider_repository_id: repository.provider_repository_id,
  };
}

function identityFieldsPresent(value: {
  host?: string;
  owner?: string;
  repo?: string;
  id?: number;
  node?: string;
}): boolean {
  return Object.values(value).some((field) => field !== undefined);
}

function githubSource(pr: TaskPR): ExternalVcsRepositoryRef | null {
  const sourceFields = identityFieldsPresent({
    host: pr.head_host,
    owner: pr.head_owner,
    repo: pr.head_repo,
    id: pr.head_repo_id,
    node: pr.head_repo_node_id,
  });
  if (!sourceFields || !pr.head_host || !pr.head_owner || !pr.head_repo) return null;
  return {
    repository: {
      provider: "github",
      provider_host: pr.head_host,
      provider_owner: pr.head_owner,
      provider_name: pr.head_repo,
      provider_repository_id:
        pr.head_repo_id !== undefined ? String(pr.head_repo_id) : pr.head_repo_node_id,
    },
    ref: pr.head_branch,
  };
}

function githubBase(pr: TaskPR): ExternalVcsRepositoryRef | null {
  const baseFields = identityFieldsPresent({
    host: pr.base_host,
    owner: pr.base_owner,
    repo: pr.base_repo,
    id: pr.base_repo_id,
  });
  if (!baseFields || !pr.base_host || !pr.base_owner || !pr.base_repo) return null;
  return {
    repository: {
      provider: "github",
      provider_host: pr.base_host,
      provider_owner: pr.base_owner,
      provider_name: pr.base_repo,
      provider_repository_id:
        pr.base_repo_id !== undefined ? String(pr.base_repo_id) : undefined,
    },
    ref: pr.base_branch,
  };
}

function gitlabSource(mr: TaskMR): ExternalVcsRepositoryRef | null {
  const sourceFields = [mr.source_host, mr.source_project_path, mr.source_project_id].some(
    (field) => field !== undefined,
  );
  if (!sourceFields || !mr.source_host || !mr.source_project_path) return null;
  const parts = mr.source_project_path.split("/");
  return {
    repository: {
      provider: "gitlab",
      provider_host: mr.source_host,
      provider_owner: parts.slice(0, -1).join("/"),
      provider_name: parts.at(-1) ?? "",
      provider_repository_id:
        mr.source_project_id !== undefined ? String(mr.source_project_id) : undefined,
    },
    ref: mr.head_branch,
  };
}

function gitlabBase(mr: TaskMR): ExternalVcsRepositoryRef | null {
  const baseFields = [mr.target_host, mr.target_project_path, mr.target_project_id].some(
    (field) => field !== undefined,
  );
  if (!baseFields || !mr.target_host || !mr.target_project_path) return null;
  const parts = mr.target_project_path.split("/");
  return {
    repository: {
      provider: "gitlab",
      provider_host: mr.target_host,
      provider_owner: parts.slice(0, -1).join("/"),
      provider_name: parts.at(-1) ?? "",
      provider_repository_id:
        mr.target_project_id !== undefined ? String(mr.target_project_id) : undefined,
    },
    ref: mr.base_branch,
  };
}

function azureOrganization(value: string | undefined): string | null {
  if (!value) return null;
  try {
    const parsed = new URL(value.includes("://") ? value : `https://${value}`);
    const [organization] = parsed.pathname.split("/").filter(Boolean);
    return organization ?? (parsed.hostname === "dev.azure.com" ? null : parsed.hostname);
  } catch {
    return null;
  }
}

function azureHost(value: string | undefined, organization: string): string {
  try {
    const parsed = value
      ? new URL(value.includes("://") ? value : `https://${value}`)
      : null;
    return `${parsed?.origin ?? "https://dev.azure.com"}/${organization}`;
  } catch {
    return `https://dev.azure.com/${organization}`;
  }
}

function azureRepository(
  organizationURL: string | undefined,
  project: string | undefined,
  name: string | undefined,
  id: string | undefined,
): ExternalVcsRepositoryRef["repository"] | null {
  const organization = azureOrganization(organizationURL);
  if (!organization || !project || !name) return null;
  return {
    provider: "azure_devops",
    provider_host: azureHost(organizationURL, organization),
    provider_organization: organization,
    provider_owner: project,
    provider_project: project,
    provider_name: name,
    provider_repository_id: id,
  };
}

function azureSource(pr: AzureDevOpsTaskPullRequest): ExternalVcsRepositoryRef | null {
  return (
    azureRepository(
      pr.sourceOrganizationUrl,
      pr.sourceProjectName,
      pr.sourceRepositoryName,
      pr.sourceRepositoryId,
    ) && {
      repository: azureRepository(
        pr.sourceOrganizationUrl,
        pr.sourceProjectName,
        pr.sourceRepositoryName,
        pr.sourceRepositoryId,
      )!,
      ref: pr.sourceBranch,
    }
  );
}

function azureBase(pr: AzureDevOpsTaskPullRequest): ExternalVcsRepositoryRef | null {
  const repository = azureRepository(
    pr.targetOrganizationUrl,
    pr.targetProjectName,
    pr.targetRepositoryName,
    pr.targetRepositoryId,
  );
  return repository ? { repository, ref: pr.targetBranch } : null;
}

function normalizeOrigin(value: string | undefined): string {
  try {
    return new URL(value ?? "").origin.toLowerCase();
  } catch {
    return "";
  }
}

function githubMatches(pr: TaskPR, repository: Repository): boolean {
  if (pr.repository_id) return pr.repository_id === repository.id;
  return repository.provider === "github" && pr.owner === repository.provider_owner && pr.repo === repository.provider_name;
}

function gitlabMatches(mr: TaskMR, repository: Repository): boolean {
  if (mr.repository_id) return mr.repository_id === repository.id;
  return repository.provider === "gitlab" &&
    normalizeOrigin(mr.host) === normalizeOrigin(repository.provider_host) &&
    mr.project_path === `${repository.provider_owner}/${repository.provider_name}`;
}

function candidateForGitHub(pr: TaskPR): LinkCandidate {
  return {
    item: pr,
    source: githubSource(pr),
    base: githubBase(pr),
    sourceFields: Object.values({ pr: pr.head_host, owner: pr.head_owner, repo: pr.head_repo, id: pr.head_repo_id, node: pr.head_repo_node_id }).some((field) => field !== undefined),
    baseFields: Object.values({ host: pr.base_host, owner: pr.base_owner, repo: pr.base_repo, id: pr.base_repo_id }).some((field) => field !== undefined),
  };
}

function candidateForGitLab(mr: TaskMR): LinkCandidate {
  return {
    item: mr,
    source: gitlabSource(mr),
    base: gitlabBase(mr),
    sourceFields: [mr.source_host, mr.source_project_path, mr.source_project_id].some((field) => field !== undefined),
    baseFields: [mr.target_host, mr.target_project_path, mr.target_project_id].some((field) => field !== undefined),
  };
}

function candidateForAzure(pr: AzureDevOpsTaskPullRequest): LinkCandidate {
  return {
    item: pr,
    source: azureSource(pr),
    base: azureBase(pr),
    sourceFields: [pr.sourceOrganizationUrl, pr.sourceProjectId, pr.sourceProjectName, pr.sourceRepositoryId, pr.sourceRepositoryName].some((field) => field !== undefined),
    baseFields: [pr.targetOrganizationUrl, pr.targetProjectId, pr.targetProjectName, pr.targetRepositoryId, pr.targetRepositoryName].some((field) => field !== undefined),
  };
}

export function providerCandidates(
  repository: Repository,
  snapshot: ProviderCandidateSnapshot,
): LinkCandidate[] {
  if (repository.provider === "github") {
    return snapshot.githubPRs.filter((pr) => githubMatches(pr, repository)).map(candidateForGitHub);
  }
  if (repository.provider === "gitlab") {
    return snapshot.gitlabMRs.filter((mr) => gitlabMatches(mr, repository)).map(candidateForGitLab);
  }
  if (repository.provider === "azure_devops") {
    return snapshot.azurePRs.filter((pr) => pr.repositoryId === repository.id).map(candidateForAzure);
  }
  return [];
}
