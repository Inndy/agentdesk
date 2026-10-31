import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = fileURLToPath(new URL("../", import.meta.url));
const all = process.argv.includes("--all");
const targets = all
  ? ["windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
  : [`${process.platform === "win32" ? "windows" : process.platform}/${process.arch === "x64" ? "amd64" : process.arch}`];
execFileSync("vp", ["run", "build"], { cwd: path.join(root, "agentdesk-ui"), stdio: "inherit" });
mkdirSync(path.join(root, "bin"), { recursive: true });
for (const target of targets) {
  const [GOOS, GOARCH] = target.split("/");
  const name = all ? `agentdesk-${GOOS}-${GOARCH}` : "agentdesk";
  const output = path.join(root, "bin", name + (GOOS === "windows" ? ".exe" : ""));
  execFileSync("go", ["build", "-trimpath", "-o", output, "./cmd/agentdesk"], {
    cwd: root, stdio: "inherit", env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: "0" },
  });
  console.log(`Built ${target}`);
}
