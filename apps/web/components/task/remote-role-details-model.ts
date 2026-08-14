import type {
  GitComparisonStatus,
  GitRemoteRefObservation,
} from "@/lib/state/slices/session-runtime/types";

export type RemoteRoleKey = "action_head" | "tracking_upstream" | "comparison_target";

export type RemoteRoleDetailsStatus = {
  actionHead?: GitRemoteRefObservation | null;
  trackingUpstream?: GitRemoteRefObservation | null;
  comparison?: GitComparisonStatus | null;
};

export type RemoteRoleRow = {
  role: RemoteRoleKey;
  state: string;
  repository: string | null;
  ref: string | null;
};

type RepositoryIdentity = {
  provider?: string;
  host?: string;
  repository_path?: string;
  provider_repository_id?: string;
};

export function formatRemoteRepository(repository: RepositoryIdentity | undefined): string | null {
  if (!repository) return null;
  const host = repository.host?.replace(/^https?:\/\//i, "").replace(/\/$/, "");
  const path = repository.repository_path?.replace(/^\/+|\/+$/g, "");
  const providerID = repository.provider_repository_id?.trim();
  const location = [host, path].filter(Boolean).join("/");
  if (location && providerID) return `${location} · ${providerID}`;
  return location || providerID || null;
}

function observationRow(
  role: Exclude<RemoteRoleKey, "comparison_target">,
  observation: GitRemoteRefObservation | null | undefined,
): RemoteRoleRow {
  return {
    role,
    state: observation?.observation_state ?? "unknown",
    repository: formatRemoteRepository(observation?.identity?.repository),
    ref: observation?.identity?.ref ?? null,
  };
}

export function buildRemoteRoleRows(status: RemoteRoleDetailsStatus): RemoteRoleRow[] {
  const target = status.comparison?.target;
  return [
    observationRow("action_head", status.actionHead),
    observationRow("tracking_upstream", status.trackingUpstream),
    {
      role: "comparison_target",
      state: status.comparison?.resolution_state ?? "unknown",
      repository: formatRemoteRepository(target?.repository),
      ref: target?.ref ?? status.comparison?.resolved_ref ?? null,
    },
  ];
}
