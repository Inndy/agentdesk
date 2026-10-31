import { execFileSync, spawn } from "node:child_process";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = fileURLToPath(new URL("../", import.meta.url));
const all = process.argv.includes("--all");
const targets = all
  ? ["windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
  : [
      `${process.platform === "win32" ? "windows" : process.platform}/${process.arch === "x64" ? "amd64" : process.arch}`,
    ];
const jobsIndex = process.argv.indexOf("--jobs");
const requestedJobs = jobsIndex >= 0 ? Number(process.argv[jobsIndex + 1]) : 3;
if (!Number.isSafeInteger(requestedJobs) || requestedJobs < 1) {
  throw new Error("--jobs must be a positive integer.");
}
const jobs = Math.min(requestedJobs, targets.length);
execFileSync("vp", ["run", "build"], { cwd: path.join(root, "agentdesk-ui"), stdio: "inherit" });
mkdirSync(path.join(root, "bin"), { recursive: true });

async function buildTarget(target) {
  const [GOOS, GOARCH] = target.split("/");
  const name = all ? `agentdesk-${GOOS}-${GOARCH}` : "agentdesk";
  const output = path.join(root, "bin", name + (GOOS === "windows" ? ".exe" : ""));
  await new Promise((resolve, reject) => {
    const child = spawn("go", ["build", "-trimpath", "-o", output, "./cmd/agentdesk"], {
      cwd: root,
      stdio: "inherit",
      windowsHide: true,
      env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: "0" },
    });
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) resolve();
      else reject(new Error(`Go build failed for ${target} (${signal ?? code}).`));
    });
  });
  console.log(`Built ${target}`);
}

console.log(`Building ${targets.length} target(s), up to ${jobs} parallel Go build(s).`);
let cursor = 0;
let failure;
async function worker() {
  while (!failure && cursor < targets.length) {
    const target = targets[cursor++];
    try {
      await buildTarget(target);
    } catch (error) {
      failure ??= error;
    }
  }
}
await Promise.all(Array.from({ length: jobs }, worker));
if (failure) throw failure;
