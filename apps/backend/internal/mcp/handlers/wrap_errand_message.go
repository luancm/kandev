package handlers

import (
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// wrapMessageForTarget wraps a message for its target session. An errand
// reports through its final message in each turn, so a parent's message to an
// errand carries that result contract in place of the sibling reply hint.
func wrapMessageForTarget(
	prompt string, senderTask *models.Task, target *models.TaskSession,
	senderSessionID, senderSessionName string, siblingSession bool,
) (string, map[string]interface{}) {
	if models.IsErrandSession(target) {
		return wrapErrandParentMessage(prompt, senderTask, senderSessionID, senderSessionName)
	}
	return wrapAgentMessage(prompt, senderTask, senderSessionID, senderSessionName, siblingSession)
}

// wrapErrandParentMessage decorates a parent's message to an errand with the
// same attribution metadata as wrapAgentMessage.
func wrapErrandParentMessage(
	prompt string, senderTask *models.Task, senderSessionID, senderSessionName string,
) (string, map[string]interface{}) {
	safeTitle := stripSystemTag(senderTask.Title)
	safeTaskID := stripSystemTag(senderTask.ID)
	safeSessionID := stripSystemTag(senderSessionID)
	sessionRef := fmt.Sprintf("session %s", safeSessionID)
	if safeName := stripSystemTag(senderSessionName); safeName != "" {
		sessionRef = fmt.Sprintf("session %q (%s)", safeName, safeSessionID)
	}
	body := fmt.Sprintf("This message was sent by your parent agent session (%s) on task %q (%s).\n", sessionRef, safeTitle, safeTaskID) +
		"You are an errand: the parent delegated bounded work to you. Treat this message as the parent's instruction for that work. " +
		"Your result is your final message in this turn; the parent receives it there. " +
		"Do not reply with message_task_kandev, do not ask the user, and do not start unrelated work."
	wrapped := sysprompt.Wrap(body) + "\n\n" + prompt
	meta := orchestrator.NewUserMessageMeta().
		WithSenderTask(senderTask.ID, senderTask.Title, senderSessionID).
		WithSenderSessionName(senderSessionName).
		ToMap()
	return wrapped, meta
}
