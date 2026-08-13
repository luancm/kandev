export type ExternalVcsProvider = "github" | "gitlab" | "azure_devops";

export type ExternalVcsRepository = {
  provider: string;
  provider_host?: string;
  provider_owner?: string;
  provider_name?: string;
  /** Azure DevOps keeps organization/project separate from repository name. */
  provider_organization?: string;
  provider_project?: string;
  provider_repository_id?: string;
  remote_url?: string;
};

export type ExternalVcsRepositoryRef = {
  repository: ExternalVcsRepository;
  ref?: string;
  revision?: string;
};

export type ExternalVcsFileURLInput = {
  repository: ExternalVcsRepository;
  path: string;
  previousPath?: string | null;
  status?: string | null;
  publishedBranch?: string | null;
  baseBranch?: string | null;
  /** Exact persisted provider identities. These must be complete as a pair. */
  sourceRepository?: ExternalVcsRepository | null;
  sourceBranch?: string | null;
  sourceRef?: string | null;
  baseRepository?: ExternalVcsRepository | null;
  baseRef?: string | null;
  /** Accepted comparison identity for an unlinked base-side file. */
  comparisonRepository?: ExternalVcsRepository | null;
  comparisonBranch?: string | null;
  comparisonRef?: string | null;
  /** Aliased shape used by status/provider callers. */
  source?: ExternalVcsRepositoryRef | null;
  base?: ExternalVcsRepositoryRef | null;
  comparison?: ExternalVcsRepositoryRef | null;
};

export type ExternalVcsFileURL = {
  provider: ExternalVcsProvider;
  url: string;
  path: string;
  revision: string;
};

type ResolvedTarget = { path: string; revision: string };
type SSHCloneIdentity = { hostname: string; parts: string[] };

type TargetRepository = {
  repository: ExternalVcsRepository;
  revision: string;
};

function cleanValue(value: string | null | undefined): string {
  return value?.trim() ?? "";
}

function normalizedProvider(value: string): string {
  return value.toLowerCase() === "azure_repos" ? "azure_devops" : value.toLowerCase();
}

function isSafeRef(value: string): boolean {
  return value.length > 0 && !/[\u0000-\u001f\u007f]/.test(value);
}

function isSafeRepositoryPath(value: string): boolean {
  if (
    !value ||
    value.startsWith("/") ||
    value.startsWith("\\") ||
    value.includes("\\") ||
    /[\u0000-\u001f\u007f]/.test(value)
  ) {
    return false;
  }
  const segments = value.split("/");
  return segments.every((segment) => segment !== "" && segment !== "." && segment !== "..");
}

function selectTarget(input: ExternalVcsFileURLInput): (ResolvedTarget & TargetRepository) | null {
  const refValue = (value: ExternalVcsRepositoryRef | null): string => value?.ref ?? value?.revision ?? "";
  const status = cleanValue(input.status).toLowerCase();
  const source = input.source ??
    (input.sourceRepository && (input.sourceBranch || input.sourceRef)
      ? { repository: input.sourceRepository, ref: input.sourceBranch || input.sourceRef || undefined }
      : null);
  const base = input.base ??
    (input.baseRepository && (input.baseBranch || input.baseRef)
      ? { repository: input.baseRepository, ref: input.baseBranch || input.baseRef || undefined }
      : null);
  const comparison = input.comparison ??
    (input.comparisonRepository && (input.comparisonBranch || input.comparisonRef)
      ? { repository: input.comparisonRepository, ref: input.comparisonBranch || input.comparisonRef || undefined }
      : null);
  if (
    (input.source != null || input.sourceRepository != null) &&
    !source &&
    status !== "deleted" &&
    !(status === "renamed" && !refValue(source))
  ) return null;
  if ((input.base != null || input.baseRepository != null) && !base && (status === "deleted" || status === "renamed" || !source)) return null;
  if ((input.comparison != null || input.comparisonRepository != null) && !comparison) {
    // A caller explicitly supplied an incomplete comparison identity. Do not
    // silently fall back to the attached repository.
    return null;
  }
  const legacyBase =
    !base && !comparison && input.baseBranch
      ? { repository: input.repository, ref: input.baseBranch }
      : null;
  const publishedBranch = cleanValue(refValue(source) || input.publishedBranch);
  const baseBranch = cleanValue(refValue(base) || refValue(comparison) || input.baseBranch || input.baseRef);
  const currentPath = input.path;
  const previousPath = input.previousPath ?? "";
  if (status === "deleted") {
    const target = base ?? comparison ?? legacyBase;
    return target && baseBranch
      ? { path: currentPath, revision: baseBranch, repository: target.repository }
      : null;
  }
  if (status === "renamed") {
    if (publishedBranch) {
      return source
        ? { path: currentPath, revision: publishedBranch, repository: source.repository }
        : input.publishedBranch
          ? { path: currentPath, revision: publishedBranch, repository: input.repository }
          : null;
    }
    const target = base ?? comparison ?? legacyBase;
    return baseBranch && previousPath && target
      ? { path: previousPath, revision: baseBranch, repository: target.repository }
      : null;
  }
  if ((status === "added" || status === "untracked") && !publishedBranch) return null;
  if (publishedBranch) {
    const target = source ?? (input.publishedBranch ? { repository: input.repository, ref: publishedBranch } : null);
    return target ? { path: currentPath, revision: publishedBranch, repository: target.repository } : null;
  }
  const target = base ?? comparison ?? legacyBase;
  return target && baseBranch
    ? { path: currentPath, revision: baseBranch, repository: target.repository }
    : null;
}

function parseHTTPSRemote(rawRemoteURL: string | undefined): URL | null {
  if (!rawRemoteURL || /[\u0000-\u001f\u007f]/.test(rawRemoteURL)) return null;
  try {
    const remote = new URL(cleanValue(rawRemoteURL));
    if (
      remote.protocol !== "https:" ||
      remote.username ||
      remote.password ||
      remote.search ||
      remote.hash
    ) {
      return null;
    }
    return remote;
  } catch {
    return null;
  }
}

function decodeSSHPath(rawPath: string): string[] | null {
  try {
    const rawParts = rawPath.replace(/^\/+/, "").split("/");
    if (rawParts.some((part) => !part)) return null;
    const parts = rawParts.map((part) => decodeURIComponent(part));
    const lastIndex = parts.length - 1;
    parts[lastIndex] = parts[lastIndex].replace(/\.git$/, "");
    if (
      parts.some(
        (part) =>
          !part ||
          part === "." ||
          part === ".." ||
          /[\\/\u0000-\u001f\u007f]/.test(part) ||
          /%[0-9a-f]{2}/i.test(part),
      )
    ) {
      return null;
    }
    return parts;
  } catch {
    return null;
  }
}

function hasValidSSHPort(value: string): boolean {
  const authorityEnd = value.indexOf("/", "ssh://".length);
  if (authorityEnd < 0) return false;
  const authority = value.slice("ssh://".length, authorityEnd);
  const hostnameAndPort = authority.slice(authority.lastIndexOf("@") + 1);
  const separator = hostnameAndPort.lastIndexOf(":");
  if (separator < 0) return true;
  const port = hostnameAndPort.slice(separator + 1);
  if (!/^\d+$/.test(port)) return false;
  const numericPort = Number(port);
  return numericPort >= 1 && numericPort <= 65535;
}

function parseSSHCloneIdentity(rawRemoteURL: string | undefined): SSHCloneIdentity | null {
  if (!rawRemoteURL || /[\u0000-\u001f\u007f]/.test(rawRemoteURL)) return null;
  const value = cleanValue(rawRemoteURL);
  const scpMatch = /^git@([A-Za-z0-9.-]+):([^?#]+)$/.exec(value);
  if (scpMatch) {
    const parts = decodeSSHPath(scpMatch[2]);
    return parts ? { hostname: scpMatch[1].toLowerCase(), parts } : null;
  }

  try {
    const remote = new URL(value);
    if (
      remote.protocol !== "ssh:" ||
      remote.username !== "git" ||
      remote.password ||
      remote.search ||
      remote.hash ||
      !hasValidSSHPort(value)
    ) {
      return null;
    }
    const parts = decodeSSHPath(remote.pathname);
    return parts ? { hostname: remote.hostname.toLowerCase(), parts } : null;
  } catch {
    return null;
  }
}

function decodedRemoteParts(remote: URL): string[] | null {
  try {
    const path = remote.pathname.replace(/\/+$/, "").replace(/\.git$/, "");
    return path
      .split("/")
      .filter(Boolean)
      .map((part) => decodeURIComponent(part));
  } catch {
    return null;
  }
}

function parseProviderOrigin(rawProviderHost: string | undefined): string | null {
  if (!rawProviderHost || /[\u0000-\u001f\u007f]/.test(rawProviderHost)) return null;
  try {
    const value = cleanValue(rawProviderHost);
    const host = new URL(value.includes("://") ? value : `https://${value}`);
    if (
      host.protocol !== "https:" ||
      host.username ||
      host.password ||
      host.search ||
      host.hash ||
      !/^\/?$/.test(host.pathname)
    ) {
      return null;
    }
    return host.origin;
  } catch {
    return null;
  }
}

function providerMetadataComplete(repository: ExternalVcsRepository): boolean {
  const provider = normalizedProvider(repository.provider);
  if (provider === "azure_devops") {
    return Boolean(
      repository.provider_name &&
        (repository.provider_project || repository.provider_owner) &&
        (repository.provider_organization || repository.provider_host || repository.remote_url),
    );
  }
  return Boolean(repository.provider_owner && repository.provider_name);
}

function azureOrganization(repository: ExternalVcsRepository): string | null {
  const explicit = cleanValue(repository.provider_organization);
  if (explicit) {
    try {
      const parsed = new URL(explicit.includes("://") ? explicit : `https://${explicit}`);
      const [organization] = parsed.pathname.split("/").filter(Boolean);
      return organization ?? (parsed.hostname === "dev.azure.com" ? null : parsed.hostname);
    } catch {
      return explicit.replace(/^https?:\/\//, "").replace(/\/+$/, "");
    }
  }
  const host = cleanValue(repository.provider_host);
  if (!host) return null;
  try {
    const parsed = new URL(host.includes("://") ? host : `https://${host}`);
    const [organization] = parsed.pathname.split("/").filter(Boolean);
    return organization ?? parsed.hostname === "dev.azure.com" ? organization ?? null : null;
  } catch {
    return null;
  }
}

function repositoryPathMatches(parts: string[], owner: string, name: string): boolean {
  const expected = owner.split("/").concat(name);
  return (
    expected.every(
      (part) =>
        part &&
        part !== "." &&
        part !== ".." &&
        !/[\\\u0000-\u001f\u007f]/.test(part) &&
        !/%[0-9a-f]{2}/i.test(part),
    ) &&
    parts.length === expected.length &&
    parts.every((part, index) => part === expected[index])
  );
}

function sshRemoteForRepository(repository: ExternalVcsRepository): URL | null {
  const identity = parseSSHCloneIdentity(repository.remote_url);
  if (!identity) return null;
  if (!repository.provider_owner || !repository.provider_name) return null;
  const provider = normalizedProvider(repository.provider);

  if (
    provider === "github" &&
    identity.hostname === "github.com" &&
    repositoryPathMatches(identity.parts, repository.provider_owner, repository.provider_name)
  ) {
    return new URL(
      `https://github.com/${encodeRepositoryPath(repository.provider_owner, repository.provider_name)}`,
    );
  }

  if (provider === "gitlab") {
    const origin = parseProviderOrigin(repository.provider_host);
    if (
      origin &&
      identity.hostname === new URL(origin).hostname.toLowerCase() &&
      repositoryPathMatches(identity.parts, repository.provider_owner, repository.provider_name)
    ) {
      return new URL(
        `${origin}/${encodeRepositoryPath(repository.provider_owner, repository.provider_name)}`,
      );
    }
  }

  if (
    provider === "azure_devops" &&
    identity.hostname === "ssh.dev.azure.com" &&
    identity.parts.length === 4 &&
    identity.parts[0] === "v3" &&
    identity.parts[2] === repository.provider_owner &&
    identity.parts[3] === repository.provider_name
  ) {
    const organization = encodeURIComponent(identity.parts[1]);
    const project = encodeURIComponent(repository.provider_owner);
    const name = encodeURIComponent(repository.provider_name);
    return new URL(`https://dev.azure.com/${organization}/${project}/_git/${name}`);
  }
  return null;
}

function parseRepositoryRemote(repository: ExternalVcsRepository): URL | null {
  return parseHTTPSRemote(repository.remote_url) ?? sshRemoteForRepository(repository);
}

function githubRemoteMatches(
  repository: ExternalVcsRepository,
  remote: URL,
  parts: string[],
): boolean {
  if (!repository.provider_owner || !repository.provider_name) return false;
  const origin = parseProviderOrigin(repository.provider_host) ?? "https://github.com";
  return (
    remote.origin === origin &&
    origin === "https://github.com" &&
    repositoryPathMatches(parts, repository.provider_owner, repository.provider_name)
  );
}

function gitlabRemoteMatches(
  repository: ExternalVcsRepository,
  remote: URL,
  parts: string[],
): boolean {
  if (!repository.provider_owner || !repository.provider_name) return false;
  const origin = parseProviderOrigin(repository.provider_host);
  return Boolean(
    origin &&
    remote.origin === origin &&
    repositoryPathMatches(parts, repository.provider_owner, repository.provider_name),
  );
}

function azureRemoteMatches(
  repository: ExternalVcsRepository,
  remote: URL,
  parts: string[],
): boolean {
  const expectedOrganization = azureOrganization(repository);
  return (
    remote.hostname === "dev.azure.com" &&
    parts.length === 4 &&
    (!expectedOrganization || parts[0] === expectedOrganization) &&
    parts[1] === repository.provider_owner &&
    parts[2] === "_git" &&
    parts[3] === repository.provider_name
  );
}

function resolveProvider(
  repository: ExternalVcsRepository,
  remote: URL,
): ExternalVcsProvider | null {
  const provider = normalizedProvider(repository.provider);
  const parts = decodedRemoteParts(remote);
  if (!parts || !providerMetadataComplete(repository)) return null;

  if (provider === "github" && githubRemoteMatches(repository, remote, parts)) return "github";
  if (provider === "gitlab" && gitlabRemoteMatches(repository, remote, parts)) return "gitlab";
  if (provider === "azure_devops" && azureRemoteMatches(repository, remote, parts)) {
    return "azure_devops";
  }
  return null;
}

function metadataWebURL(
  repository: ExternalVcsRepository,
): { provider: ExternalVcsProvider; webURL: URL } | null {
  const provider = normalizedProvider(repository.provider);
  if (!providerMetadataComplete(repository)) return null;
  if (provider === "github") {
    const origin = parseProviderOrigin(repository.provider_host) ?? "https://github.com";
    if (origin !== "https://github.com") return null;
    return {
      provider: "github",
      webURL: new URL(`${origin}/${encodeRepositoryPath(repository.provider_owner!, repository.provider_name!)}`),
    };
  }
  if (provider === "gitlab") {
    const origin = parseProviderOrigin(repository.provider_host);
    if (!origin) return null;
    return {
      provider: "gitlab",
      webURL: new URL(`${origin}/${encodeRepositoryPath(repository.provider_owner!, repository.provider_name!)}`),
    };
  }
  if (provider === "azure_devops") {
    const organization = azureOrganization(repository);
    const project = repository.provider_project || repository.provider_owner;
    if (!organization || !project || !repository.provider_name) return null;
    return {
      provider: "azure_devops",
      webURL: new URL(
        `https://dev.azure.com/${encodeURIComponent(organization)}/${encodeURIComponent(project)}/_git/${encodeURIComponent(repository.provider_name)}`,
      ),
    };
  }
  return null;
}

function encodeRepositoryPath(owner: string, name: string): string {
  return owner
    .split("/")
    .concat(name)
    .map((segment) => encodeURIComponent(segment))
    .join("/");
}

function buildFileURL(
  provider: ExternalVcsProvider,
  repository: ExternalVcsRepository,
  remote: URL,
  target: ResolvedTarget,
): string {
  if (provider === "azure_devops") {
    const url = new URL(remote.toString());
    url.pathname = url.pathname.replace(/\/+$/, "").replace(/\.git$/, "");
    url.searchParams.set("path", `/${target.path}`);
    url.searchParams.set("version", `GB${target.revision}`);
    return url.toString();
  }
  const repositoryPath = encodeRepositoryPath(repository.provider_owner!, repository.provider_name!);
  const route = provider === "gitlab" ? "-/blob" : "blob";
  const filePath = target.path.split("/").map(encodeURIComponent).join("/");
  return `${remote.origin}/${repositoryPath}/${route}/${encodeURIComponent(target.revision)}/${filePath}`;
}

export function resolveExternalVcsFileURL(
  input: ExternalVcsFileURLInput,
): ExternalVcsFileURL | null {
  const target = selectTarget(input);
  const hasRemote = Boolean(target?.repository.remote_url);
  const remote = target && hasRemote ? parseRepositoryRemote(target.repository) : null;
  const resolved: { provider: ExternalVcsProvider | null; webURL: URL } | null = hasRemote
    ? remote
      ? { provider: resolveProvider(target!.repository, remote), webURL: remote }
      : null
    : target
      ? metadataWebURL(target.repository)
      : null;
  if (
    !resolved ||
    !resolved.provider ||
    !target ||
    !isSafeRepositoryPath(target.path) ||
    !isSafeRef(target.revision)
  ) {
    return null;
  }
  return {
    provider: resolved.provider,
    url: buildFileURL(resolved.provider, target.repository, resolved.webURL, target),
    path: target.path,
    revision: target.revision,
  };
}
