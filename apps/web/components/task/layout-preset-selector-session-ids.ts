type LayoutSession = { id: string };

export async function resolveLayoutApplySessionIds(args: {
  activeTaskId: string | null;
  sessionsLoaded: boolean;
  loadSessions: (force?: boolean) => Promise<unknown> | unknown;
  getSessionsForTask: (taskId: string) => readonly LayoutSession[];
  getSessionsLoadedForTask: (taskId: string) => boolean;
}): Promise<{ sessionIds: string[]; loaded: boolean }> {
  const {
    activeTaskId,
    sessionsLoaded,
    loadSessions,
    getSessionsForTask,
    getSessionsLoadedForTask,
  } = args;
  if (activeTaskId && !sessionsLoaded) await loadSessions(true);

  const loaded = activeTaskId ? getSessionsLoadedForTask(activeTaskId) : true;
  return {
    sessionIds:
      activeTaskId && loaded ? getSessionsForTask(activeTaskId).map((session) => session.id) : [],
    loaded,
  };
}
