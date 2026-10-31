import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  chmodSync,
} from "node:fs";
import { homedir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const codexDir = process.env.CODEX_HOME || path.join(homedir(), ".codex");
const destinations = [
  path.join(codexDir, "skills", "agentdesk"),
  path.join(homedir(), ".claude", "skills", "agentdesk"),
];
const name = process.platform === "win32" ? "agentdesk.exe" : "agentdesk";
const sourceBinary = path.join(root, "bin", name);
const binary = path.join(homedir(), ".local", "bin", name);
if (!existsSync(sourceBinary)) throw new Error("Build Agentdesk first with make build.");
const index = process.argv.indexOf("--telegram-config");
const telegramConfig = index >= 0 ? process.argv[index + 1] : undefined;
if (index >= 0 && !telegramConfig) throw new Error("--telegram-config requires a path.");
mkdirSync(path.dirname(binary), { recursive: true });
copyFileSync(sourceBinary, binary);
if (process.platform !== "win32") chmodSync(binary, 0o755);
for (const destination of destinations) {
  mkdirSync(destination, { recursive: true });
  copyFileSync(
    path.join(root, "skills", "agentdesk", "SKILL.md"),
    path.join(destination, "SKILL.md"),
  );
  const runtimePath = path.join(destination, "runtime.json");
  const previous = existsSync(runtimePath) ? JSON.parse(readFileSync(runtimePath, "utf8")) : {};
  writeFileSync(
    runtimePath,
    JSON.stringify(
      {
        binary,
        telegram_config: telegramConfig ? path.resolve(telegramConfig) : previous.telegram_config,
      },
      null,
      2,
    ) + "\n",
    { mode: 0o600 },
  );
  console.log(`Installed agentdesk skill at ${destination}`);
}
console.log(`Shared executable: ${binary}`);
