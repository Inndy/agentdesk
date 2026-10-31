import { chromium } from "@playwright/test";
import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
const binary =
  process.env.AGENTDESK_BINARY ||
  path.join(root, "bin", process.platform === "win32" ? "agentdesk.exe" : "agentdesk");
const fixture = JSON.parse(readFileSync(path.join(root, "examples/spicesharp-demo.json"), "utf8"));
const locale = process.env.AGENTDESK_DEMO_LOCALE === "en" ? "en" : "zh-TW";
const suffix = locale === "en" ? "-en" : "";
const dir = mkdtempSync(path.join(tmpdir(), "agentdesk-demo-"));
const output = path.join(root, "docs/images");
mkdirSync(output, { recursive: true });
function cli(args, input) {
  return JSON.parse(
    execFileSync(binary, [...args, "--data-dir", dir], {
      encoding: "utf8",
      input,
      windowsHide: true,
    }),
  ).data;
}
cli(["init", "--name", fixture.project_name]);
const service = spawn(
  binary,
  ["serve", "--listen", "127.0.0.1:0", "--telegram=false", "--data-dir", dir],
  { windowsHide: true, stdio: ["ignore", "ignore", "pipe"] },
);
let ready = false;
service.stderr.on("data", (chunk) => {
  if (String(chunk).includes("listening at")) ready = true;
});
let browser;
let context;
try {
  const deadline = Date.now() + 15000;
  while (!ready) {
    if (service.exitCode !== null || Date.now() > deadline)
      throw new Error("Demo service did not become ready.");
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  const endpoint = JSON.parse(readFileSync(path.join(dir, "project.json"), "utf8")).endpoint;
  const issues = [];
  for (const draft of [...fixture.issues].reverse()) {
    issues.unshift(cli(["create", "--file", "-", "--key", draft.title], JSON.stringify(draft)));
  }
  browser =
    process.env.CI && !process.env.AGENTDESK_CDP_URL
      ? await chromium.launch({ headless: true })
      : await chromium.connectOverCDP(process.env.AGENTDESK_CDP_URL || "http://127.0.0.1:9222");
  context = await browser.newContext({
    locale,
    viewport: { width: 1440, height: 1100 },
    colorScheme: "dark",
  });
  const page = await context.newPage();
  const credential = JSON.parse(readFileSync(path.join(dir, "human.json"), "utf8"));
  // Token stays in memory; traces, videos and secret URLs are never emitted.
  try {
    await page.goto(`${endpoint}/?issue=${issues[0].issue_id}#token=${credential.token}`);
  } catch {
    throw new Error("Could not load the isolated demo login.");
  }
  const detail = page.getByRole("region", { name: locale === "en" ? "Issue details" : "議題詳情" });
  await detail.getByRole("heading", { name: issues[0].title, exact: true }).waitFor();
  if (new URL(page.url()).hash) throw new Error("Login token was not cleared.");
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({
    path: path.join(output, `spicesharp-desktop${suffix}.png`),
    fullPage: true,
  });
  const action = issues.find((issue) => issue.kind === "action");
  await page.goto(`${endpoint}/?issue=${action.issue_id}`);
  await page.setViewportSize({ width: 390, height: 844 });
  await detail.getByRole("heading", { name: action.title, exact: true }).waitFor();
  if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth))
    throw new Error("Mobile demo overflows horizontally.");
  await page.screenshot({
    path: path.join(output, `spicesharp-mobile${suffix}.png`),
    fullPage: true,
  });
  console.log("Captured desktop/mobile demo screenshots from isolated SpiceSharp replay.");
} finally {
  await context?.close();
  await browser?.close();
  if (service.exitCode === null) {
    const closed = new Promise((resolve) => service.once("exit", resolve));
    service.kill("SIGKILL");
    await closed;
  }
}
