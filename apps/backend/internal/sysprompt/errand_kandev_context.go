package sysprompt

const (
	errandVarTaskID    = "task_id"
	errandVarSessionID = "session_id"
)

// FormatErrandKandevContext renders the task-mode context of an errand session.
// An errand sees only the read-only Kandev catalog that errand.md describes, so
// the task, plan-editing, question, and delegation tools never appear in it.
func FormatErrandKandevContext(taskID, sessionID string) string {
	return Resolve("errand-kandev-context", map[string]string{
		errandVarTaskID:    taskID,
		errandVarSessionID: sessionID,
	})
}
