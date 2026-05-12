"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { requestCommitDiff } from "@/components/task/commit-diff-request";
import type { FileInfo } from "@/lib/state/store";

type UseCommitDiffResult = {
  files: Record<string, FileInfo> | null;
  loading: boolean;
  refetch: () => Promise<void>;
};

/**
 * Fetches a commit's per-file diff via WebSocket and re-fetches once
 * agentctl transitions from not-ready to ready. Used by both desktop
 * (CommitDetailPanel) and mobile (CommitDiffView in the diff sheet).
 */
export function useCommitDiff(commitSha: string, repo?: string): UseCommitDiffResult {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionTaskId = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.task_id : undefined,
  );
  const agentctlReady = useAppStore((state) =>
    activeSessionId
      ? state.sessionAgentctl.itemsBySessionId[activeSessionId]?.status === "ready"
      : false,
  );
  const { toast } = useToast();

  const [files, setFiles] = useState<Record<string, FileInfo> | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchDiff = useCallback(async () => {
    if (!activeSessionId) return;
    setLoading(true);
    try {
      const response = await requestCommitDiff({
        sessionId: activeSessionId,
        taskId: sessionTaskId ?? activeTaskId ?? null,
        commitSha,
        agentctlReady,
        repo,
      });
      if (response?.success && response.files) {
        setFiles(response.files);
      }
    } catch (err) {
      toast({
        title: "Failed to load commit diff",
        description: err instanceof Error ? err.message : "An unexpected error occurred",
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [activeSessionId, activeTaskId, agentctlReady, commitSha, repo, sessionTaskId, toast]);

  useEffect(() => {
    fetchDiff();
  }, [fetchDiff]);

  // Re-fetch when agentctl transitions from not-ready to ready and we don't
  // yet have any files. Without this, opening the panel before agentctl is up
  // would show "No files" forever.
  const prevAgentctlReadyRef = useRef(agentctlReady);
  useEffect(() => {
    const prev = prevAgentctlReadyRef.current;
    prevAgentctlReadyRef.current = agentctlReady;
    if (!prev && agentctlReady && files === null) {
      fetchDiff();
    }
  }, [agentctlReady, files, fetchDiff]);

  return { files, loading, refetch: fetchDiff };
}
