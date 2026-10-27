KANDEV MCP TOOLS: read-only catalog of an errand session.
Kandev Task ID: {task_id}
Kandev Session ID: {session_id}
Use these IDs for task_id/session_id.

MCP DISCOVERY:
These instructions list the read-only Kandev tools of this errand session, not the complete MCP catalog.
Use callable tools from "kandev" with their current schemas; otherwise use native tool search or discovery. Search "kandev" plus the canonical name; without search, inspect available MCP tools.
An omitted entry here does not mean that the tool is unavailable. If discovery fails, report the limitation before substituting.

Available tools (read-only):
- list_workspaces_kandev: List workspaces.
- list_workflows_kandev: List the workflows of a workspace.
- list_workflow_steps_kandev: List the steps of a workflow.
- list_tasks_kandev: List tasks.
- list_related_tasks_kandev: List a task's parent, children, siblings, blockers, and blocked tasks.
- list_task_sessions_kandev: List a task's sessions and their IDs.
- get_task_conversation_kandev: Read the conversation history of a task.
- get_task_plan_kandev: Read the current plan of a task.

These tools only read Kandev state. Do not call any other Kandev tool.
