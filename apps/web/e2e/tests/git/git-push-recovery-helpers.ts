import fs from "node:fs";
import path from "node:path";

/** Installs a bare-remote hook that rejects every receive-pack transaction. */
export function installRejectingPreReceiveHook(remotePath: string): string {
  const hookPath = path.join(remotePath, "hooks", "pre-receive");
  fs.mkdirSync(path.dirname(hookPath), { recursive: true });
  fs.writeFileSync(hookPath, "#!/bin/sh\necho 'e2e remote rejection' >&2\nexit 1\n", "utf8");
  fs.chmodSync(hookPath, 0o755);
  return hookPath;
}

/** Removes the temporary receive hook so the next push can succeed. */
export function removeRejectingPreReceiveHook(hookPath: string): void {
  fs.rmSync(hookPath, { force: true });
}
