import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub PR list task indicator", () => {
  test("renders single-task button, empty badge, and tooltip with task + step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    await apiClient.mockGitHubAddPRs([
      {
        number: 100,
        title: "PR with linked task",
        state: "open",
        head_branch: "feat/linked",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
      {
        number: 101,
        title: "PR without task",
        state: "open",
        head_branch: "feat/orphan",
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const task = await apiClient.createTask(seedData.workspaceId, "Linked task title", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 100,
      pr_url: "https://github.com/testorg/testrepo/pull/100",
      pr_title: "PR with linked task",
      head_branch: "feat/linked",
      base_branch: "main",
      author_login: "test-user",
    });

    await testPage.goto("/github");

    const singleIndicator = testPage.getByTestId("pr-row-task-indicator-single");
    await expect(singleIndicator).toBeVisible({ timeout: 15_000 });
    await expect(singleIndicator).toContainText("Linked task title");

    const emptyIndicator = testPage.getByTestId("pr-row-task-indicator-empty");
    await expect(emptyIndicator).toBeVisible();
    await expect(emptyIndicator).toHaveText("No task created yet");

    await singleIndicator.hover();

    const startStep = seedData.steps.find((s) => s.id === seedData.startStepId);
    const tooltip = testPage.getByRole("tooltip");
    await expect(tooltip).toContainText("Task: Linked task title", { timeout: 5_000 });
    if (startStep?.title) {
      await expect(tooltip).toContainText(`Step: ${startStep.title}`);
    }
  });
});
