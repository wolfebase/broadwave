// Build the site into the server binary the browser tests launch.
import { spawnSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const web = path.resolve(here, "..");
const root = path.resolve(web, "..");
const run = path.join(here, ".run");
mkdirSync(run, { recursive: true });

function runStep(cmd, args, cwd) {
  const result = spawnSync(cmd, args, { cwd, stdio: "inherit" });
  if (result.status !== 0) process.exit(result.status ?? 1);
}

runStep("npm", ["run", "build"], web);
runStep("go", ["build", "-o", path.join(run, "broadwave"), "./server/cmd/broadwave"], root);
runStep("go", ["build", "-o", path.join(run, "fakehdhr"), "./server/cmd/fakehdhr"], root);
