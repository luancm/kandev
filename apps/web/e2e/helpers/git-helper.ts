import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import type { ApiClient } from "./api-client";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

export class GitHelper {
  constructor(
    private repoDir: string,
    private env: NodeJS.ProcessEnv,
  ) {}

  exec(cmd: string): string {
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        return execSync(cmd, { cwd: this.repoDir, env: this.env, encoding: "utf8" });
      } catch (err) {
        const msg = (err as Error).message ?? "";
        if (msg.includes("index.lock") && attempt < 2) {
          execSync("sleep 0.3");
          continue;
        }
        throw err;
      }
    }
    throw new Error(`git exec failed after 3 attempts: ${cmd}`);
  }

  createFile(name: string, content: string | Buffer) {
    const filePath = path.join(this.repoDir, name);
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, content);
  }

  modifyFile(name: string, content: string | Buffer) {
    this.createFile(name, content);
  }

  deleteFile(name: string) {
    const filePath = path.join(this.repoDir, name);
    if (fs.existsSync(filePath)) fs.unlinkSync(filePath);
  }

  stageFile(name: string) {
    this.exec(`git add "${name}"`);
  }

  stageAll() {
    this.exec("git add -A");
  }

  commit(message: string): string {
    this.exec(`git commit -m "${message}"`);
    return this.exec("git rev-parse HEAD").trim();
  }

  getCurrentSha(): string {
    return this.exec("git rev-parse HEAD").trim();
  }

  getRepoDir(): string {
    return this.repoDir;
  }

  getEnv(): NodeJS.ProcessEnv {
    return this.env;
  }
}

export type TriangularRemoteFixture = {
  branch: string;
  localHead: string;
  comparisonHead: string;
  trackingHead: string;
  comparisonURL: string;
  writableURL: string;
  trackingURL: string;
  unrelatedURL: string;
};

/**
 * Configure a production-shaped triangular checkout for remote-role E2E tests.
 * The remotes retain provider URLs in Git config while url.insteadOf maps them
 * to isolated local bare repositories, so agentctl resolves real provider
 * identities without requiring network access.
 */
export function configureTriangularRemoteFixture(
  git: GitHelper,
  tmpDir: string,
): TriangularRemoteFixture {
  const suffix = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  const branch = `feature/role-aware-${suffix}`;
  const remotes = {
    comparison: `https://github.com/testorg/canonical-${suffix}.git`,
    writable: `https://github.com/testorg/fork-${suffix}.git`,
    tracking: `https://github.com/testorg/tracking-${suffix}.git`,
    unrelated: `https://github.com/unrelated/unrelated-${suffix}.git`,
  };
  const barePaths = Object.fromEntries(
    Object.keys(remotes).map((name) => [name, path.join(tmpDir, "repos", `role-${name}-${suffix}.git`)]),
  ) as Record<keyof typeof remotes, string>;
  const env = git.getEnv();

  for (const barePath of Object.values(barePaths)) {
    fs.mkdirSync(path.dirname(barePath), { recursive: true });
    execSync(`git init --bare -b main "${barePath}"`, { env });
  }

  git.exec("git checkout -B role-aware-seed");
  git.createFile("role-aware-landed.txt", "already present on canonical base\n");
  git.stageFile("role-aware-landed.txt");
  const comparisonHead = git.commit("Canonical base change already landed");
  git.createFile("role-aware-local.txt", "local contribution\n");
  git.stageFile("role-aware-local.txt");
  const localHead = git.commit("Local contribution on writable branch");

  git.exec("git checkout -B role-aware-tracking");
  git.createFile("role-aware-tracking.txt", "tracking-only change\n");
  git.stageFile("role-aware-tracking.txt");
  const trackingHead = git.commit("Tracking upstream change");
  git.exec(`git checkout -B "${branch}" ${localHead}`);

  const initialHead = git.exec("git rev-list --max-parents=0 HEAD").trim();
  execSync(`git push "${barePaths.comparison}" ${comparisonHead}:refs/heads/main`, {
    cwd: git.getRepoDir(),
    env,
  });
  execSync(`git push "${barePaths.tracking}" ${trackingHead}:refs/heads/main`, {
    cwd: git.getRepoDir(),
    env,
  });
  execSync(`git push "${barePaths.writable}" ${comparisonHead}:refs/heads/${branch}`, {
    cwd: git.getRepoDir(),
    env,
  });
  execSync(`git push "${barePaths.unrelated}" ${initialHead}:refs/heads/main`, {
    cwd: git.getRepoDir(),
    env,
  });

  for (const [name, remoteURL] of Object.entries(remotes)) {
    if (git.exec("git remote").split(/\r?\n/).includes(name === "writable" ? "publish" : name)) {
      git.exec(`git remote remove "${name === "writable" ? "publish" : name}"`);
    }
    const remoteName = name === "writable" ? "publish" : name;
    git.exec(`git remote add "${remoteName}" "${remoteURL}"`);
    git.exec(`git config --local url."file://${barePaths[name as keyof typeof barePaths]}".insteadOf "${remoteURL}"`);
  }
  git.exec(`git config branch."${branch}".remote tracking`);
  git.exec(`git config branch."${branch}".merge refs/heads/main`);
  git.exec(`git config branch."${branch}".pushRemote publish`);
  git.exec("git fetch comparison main");
  git.exec("git fetch tracking main");
  git.exec(`git fetch publish "${branch}"`);
  git.exec("git fetch origin main");

  return {
    branch,
    localHead,
    comparisonHead,
    trackingHead,
    comparisonURL: remotes.comparison,
    writableURL: remotes.writable,
    trackingURL: remotes.tracking,
    unrelatedURL: remotes.unrelated,
  };
}

/**
 * Strip GIT_CONFIG_* environment variables that can inject global git hooks
 * into fresh git repos, breaking E2E test setup.
 */
function stripGitConfigOverrides(env: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  const clean: NodeJS.ProcessEnv = {};
  for (const [key, value] of Object.entries(env)) {
    if (/^GIT_CONFIG_(KEY|VALUE|COUNT|PARAMETERS|GLOBAL|SYSTEM)/i.test(key)) continue;
    clean[key] = value;
  }
  return clean;
}

export function makeGitEnv(tmpDir: string): NodeJS.ProcessEnv {
  return {
    ...stripGitConfigOverrides(process.env),
    HOME: tmpDir,
    GIT_AUTHOR_NAME: "E2E Test",
    GIT_AUTHOR_EMAIL: "e2e@test.local",
    GIT_COMMITTER_NAME: "E2E Test",
    GIT_COMMITTER_EMAIL: "e2e@test.local",
  };
}

export async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

export async function createStandardProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  const agentId = agents[0]?.id;
  if (!agentId) throw new Error("No agent available");
  return apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: false,
  });
}
