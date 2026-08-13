import type { ForegroundActivity, TaskPendingAction, TaskSessionState } from "./http";

export type TaskStatusSummary = {
  revision: number;
  updated_at: string;
  /** Semantic task activity, separate from summary projection freshness. */
  last_activity_at?: string;
  primary_session?: {
    id: string;
    state: TaskSessionState;
  } | null;
  foreground_activity?: ForegroundActivity;
  active_subagent_count?: number;
  pending_action?: TaskPendingAction;
  /** Number of prompts currently en-queued for the task (all sessions). */
  queued_prompt_count?: number;
  active_error?: {
    session_id: string;
    stamp: string;
    occurred_at: string;
    preview: string;
  } | null;
  git?: {
    additions?: number;
    deletions?: number;
    changed_files?: number;
    ahead?: number;
    behind?: number;
    comparison?: GitComparisonSummary;
    comparison_by_repository?: Record<string, GitComparisonSummary>;
  } | null;
  pull_request?: {
    count?: number;
    open_count?: number;
    attention?: boolean;
    aggregate_state?: string;
    state?: string;
    number?: number;
    url?: string;
  } | null;
};

export type GitComparisonSummary = {
  context_generation?: string;
  resolution_state?: string;
  reason?: string;
  resolved_ref?: string;
  base_commit?: string;
  target?: {
    repository: {
      provider?: string;
      host?: string;
      repository_path?: string;
      provider_repository_id?: string;
    };
    ref: string;
  };
};
