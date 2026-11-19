package handlers

import (
	"fmt"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// wrapErrandParentMessage adds the verified sender and result contract to the prompt.
func wrapErrandParentMessage(
	prompt string, senderTask *models.Task, senderSessionID, senderSessionName string,
) string {
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
	return sysprompt.Wrap(body) + "\n\n" + prompt
}
