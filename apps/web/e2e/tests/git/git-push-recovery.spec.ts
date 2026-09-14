import { test, expect } from "../../fixtures/test-base";
import {
  createEmptyRemoteRepository,
  openTaskByID,
  remoteRef,
  removeTestRepository,
  taskWorktreeGit,
  waitForTaskWorktree,
} from "../../helpers/empty-remote-repository";
import {
  installRejectingPreReceiveHook,
  removeRejectingPreReceiveHook,
} from "./git-push-recovery-helpers";

test.describe("Git push recovery", () => {
  test.setTimeout(180_000);

  test("clears failed push feedback after a successful retry and reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = createEmptyRemoteRepository(backend.tmpDir, "recovery-desktop");
    let repositoryID = "";
    let rejectingHookPath = "";

    try {
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.localPath,
        "main",
        { name: "Git Push Recovery Desktop" },
      );
      repositoryID = repository.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Git push recovery desktop",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_profile_id: seedData.worktreeExecutorProfileId,
          repositories: [{ repository_id: repositoryID, base_branch: "main" }],
        },
      );

      const session = await openTaskByID(testPage, task.id);
      await session.waitForChatIdle({ timeout: 60_000 });
      const worktreePath = await waitForTaskWorktree(apiClient, task.id, repositoryID);
      const git = taskWorktreeGit(worktreePath, fixture.gitEnv);

      git.createFile("git-push-recovery-desktop.txt", "retry the push\n");
      git.stageAll();
      const taskCommit = git.commit("Add push recovery desktop fixture");
      const taskBranch = git.exec("git branch --show-current").trim();
      rejectingHookPath = installRejectingPreReceiveHook(fixture.remotePath);

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 15_000 });
      await session.expandCommitsSection();
      const pushButton = session.changes.getByTestId("commits-repo-push");
      await expect(pushButton).toBeVisible({ timeout: 15_000 });
      await pushButton.click();

      const errorMessage = session.gitOperationErrorMessage();
      await expect(errorMessage).toBeVisible({ timeout: 30_000 });
      await expect(errorMessage.getByText("Git push failed", { exact: true })).toBeVisible();
      await expect(session.gitFixButton()).toBeVisible();

      removeRejectingPreReceiveHook(rejectingHookPath);
      rejectingHookPath = "";
      await expect(pushButton).toBeVisible({ timeout: 30_000 });
      await expect(pushButton).toBeEnabled({ timeout: 30_000 });
      await pushButton.click();

      await expect(
        testPage.getByTestId("toast-message").filter({ hasText: "Push successful" }),
      ).toBeVisible({ timeout: 30_000 });
      await expect
        .poll(() => remoteRef(git, taskBranch), {
          timeout: 30_000,
          message: "Expected the retried push to publish the task branch",
        })
        .toBe(taskCommit);
      await expect(session.gitFixButton()).toHaveCount(0, { timeout: 30_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(0);

      await testPage.reload();
      await session.waitForLoad();
      await expect(session.gitFixButton()).toHaveCount(0, { timeout: 15_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(0);
    } finally {
      if (rejectingHookPath) removeRejectingPreReceiveHook(rejectingHookPath);
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]).catch(() => undefined);
      if (repositoryID) await removeTestRepository(apiClient, repositoryID);
      fixture.cleanup();
    }
  });

  test("clears failed push feedback after an external terminal push and reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = createEmptyRemoteRepository(backend.tmpDir, "recovery-terminal");
    let repositoryID = "";
    let rejectingHookPath = "";

    try {
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.localPath,
        "main",
        { name: "Git Push Recovery Terminal" },
      );
      repositoryID = repository.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Git push recovery terminal",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_profile_id: seedData.worktreeExecutorProfileId,
          repositories: [{ repository_id: repositoryID, base_branch: "main" }],
        },
      );
      if (!task.session_id) {
        throw new Error("Git push recovery terminal task did not return a session");
      }

      const session = await openTaskByID(testPage, task.id);
      await session.waitForChatIdle({ timeout: 60_000 });
      const worktreePath = await waitForTaskWorktree(apiClient, task.id, repositoryID);
      const git = taskWorktreeGit(worktreePath, fixture.gitEnv);

      git.createFile("git-push-recovery-terminal.txt", "retry the push\n");
      git.stageAll();
      const taskCommit = git.commit("Add push recovery terminal fixture");
      const taskBranch = git.exec("git branch --show-current").trim();
      rejectingHookPath = installRejectingPreReceiveHook(fixture.remotePath);

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible({ timeout: 15_000 });
      await session.expandCommitsSection();
      const pushButton = session.changes.getByTestId("commits-repo-push");
      await expect(pushButton).toBeVisible({ timeout: 15_000 });
      await pushButton.click();

      const errorMessage = session.gitOperationErrorMessage();
      await expect(errorMessage).toBeVisible({ timeout: 30_000 });
      await expect(errorMessage.getByText("Git push failed", { exact: true })).toBeVisible();
      await expect(session.gitFixButton()).toBeVisible();

      await apiClient.seedSessionMessage(task.session_id, {
        type: "error",
        content: "Git push failed",
        metadata: {
          git_operation_error: true,
          operation: "push",
          error_output: "e2e legacy remote rejection",
          variant: "error",
          actions: [
            {
              type: "ws_request",
              label: "Fix",
              icon: "sparkles",
              tooltip: "Ask the agent to fix the git error",
              test_id: "git-fix-button",
              params: {
                method: "message.add",
                payload: {
                  task_id: task.id,
                  session_id: task.session_id,
                  content: "Please fix the legacy git push error.",
                },
              },
            },
          ],
        },
      });

      await testPage.reload();
      await session.waitForLoad();
      await expect(session.gitFixButton()).toHaveCount(2, { timeout: 15_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(2);

      removeRejectingPreReceiveHook(rejectingHookPath);
      rejectingHookPath = "";
      git.exec(`git push --set-upstream origin ${taskBranch}`);

      await expect
        .poll(() => remoteRef(git, taskBranch), {
          timeout: 30_000,
          message: "Expected the terminal push to publish the task branch",
        })
        .toBe(taskCommit);
      await expect(session.gitFixButton()).toHaveCount(0, { timeout: 30_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(0);

      await testPage.reload();
      await session.waitForLoad();
      await expect(session.gitFixButton()).toHaveCount(0, { timeout: 15_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(0);
    } finally {
      if (rejectingHookPath) removeRejectingPreReceiveHook(rejectingHookPath);
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]).catch(() => undefined);
      if (repositoryID) await removeTestRepository(apiClient, repositoryID);
      fixture.cleanup();
    }
  });
});
