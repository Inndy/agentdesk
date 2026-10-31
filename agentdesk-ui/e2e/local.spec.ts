import {
  chromium,
  expect,
  test,
  type Browser,
  type BrowserContext,
  type Page,
} from "@playwright/test";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const binary = path.resolve("../bin", process.platform === "win32" ? "agentdesk.exe" : "agentdesk");
let browser: Browser;
let context: BrowserContext;
let page: Page;
let service: ChildProcess;
let dir: string;
let endpoint: string;

function cli(...args: string[]) {
  return JSON.parse(
    execFileSync(binary, [...args, "--data-dir", dir], { encoding: "utf8", windowsHide: true }),
  ).data;
}

function create(kind: string, title: string, options = false) {
  const draft = {
    kind,
    title,
    context: "這是本機端到端驗證，不涉及外部操作。",
    blocking_scope: "只有這個測試工作需要等待",
    created_by: "e2e-agent",
    options: options
      ? [
          { id: "small", label: "小批次", pros: "容易驗證", cons: "次數較多" },
          { id: "large", label: "大批次" },
        ]
      : undefined,
    recommended_option: options ? "large" : undefined,
  };
  return JSON.parse(
    execFileSync(binary, ["create", "--file", "-", "--key", title, "--json", "--data-dir", dir], {
      input: JSON.stringify(draft),
      encoding: "utf8",
      windowsHide: true,
    }),
  ).data;
}

async function startService() {
  let ready = false;
  service = spawn(binary, ["serve", "--listen", "127.0.0.1:0", "--data-dir", dir], {
    windowsHide: true,
    stdio: ["ignore", "ignore", "pipe"],
  });
  service.stderr?.on("data", (chunk) => {
    if (String(chunk).includes("listening at")) ready = true;
  });
  await expect
    .poll(
      async () => {
        if (service.exitCode !== null) throw new Error("test service exited before readiness");
        if (!ready) return false;
        try {
          endpoint = JSON.parse(readFileSync(path.join(dir, "project.json"), "utf8")).endpoint;
          return (await fetch(endpoint)).ok;
        } catch {
          return false;
        }
      },
      { timeout: 15000 },
    )
    .toBe(true);
  expect(Number(readFileSync(path.join(dir, "server.pid"), "utf8").trim())).toBe(service.pid);
}

async function stopService() {
  if (service && service.exitCode === null) {
    const closed = new Promise<void>((resolve) => service.once("exit", () => resolve()));
    service.kill("SIGKILL");
    await closed;
  }
}

async function login() {
  const credential = JSON.parse(readFileSync(path.join(dir, "human.json"), "utf8"));
  // Simulate opening a login link in a fresh document, even if already at /.
  await page.goto("about:blank");
  // Catch navigation errors without including the secret URL in the test report.
  try {
    await page.goto(`${endpoint}/#token=${credential.token}`);
  } catch {
    throw new Error("could not navigate to test service login");
  }
  await expect(page.getByRole("heading", { name: "收件匣", exact: true })).toBeVisible();
  expect(new URL(page.url()).hash).toBe("");
}

test.beforeAll(async () => {
  browser = await chromium.connectOverCDP(process.env.AGENTDESK_CDP_URL ?? "http://127.0.0.1:9222");
});
test.beforeEach(async () => {
  dir = mkdtempSync(path.join(tmpdir(), "agentdesk-e2e-"));
  cli("init", "--name", "E2E 專案");
  await startService();
  context = await browser.newContext({
    viewport: test.info().title.includes("手機")
      ? { width: 390, height: 844 }
      : { width: 1440, height: 1000 },
  });
  page = await context.newPage();
});
test.afterEach(async () => {
  await context?.close();
  await stopService();
});
test.afterAll(async () => {
  await browser?.close();
});

test("選項回答、CLI wait、receipt 與服務重啟", async () => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const issue = create("decision", "選擇測試批次", true);
  expect(create("decision", "選擇測試批次", true).issue_id).toBe(issue.issue_id);
  await login();
  await page.getByRole("button", { name: /選擇測試批次/ }).click();
  const radios = page.getByRole("radio");
  await expect(radios.nth(0)).not.toBeChecked();
  await expect(radios.nth(1)).not.toBeChecked();
  await expect(page.getByRole("button", { name: "送出回答", exact: true })).toBeDisabled();
  const waiter = spawn(
    binary,
    ["wait", issue.issue_id, "--timeout", "10s", "--json", "--data-dir", dir],
    { windowsHide: true },
  );
  let waitOutput = "";
  waiter.stdout.on("data", (chunk) => {
    waitOutput += chunk;
  });
  const waitDone = new Promise<number | null>((resolve) => waiter.once("exit", resolve));
  await radios.nth(0).check();
  await page.getByLabel("你的回答或補充說明").fill("先做小批次，僅限這項測試。");
  await page.getByRole("button", { name: "送出回答", exact: true }).click();
  await expect(page.getByRole("heading", { name: "已保存你的回答" })).toBeVisible();
  expect(await waitDone).toBe(0);
  expect(JSON.parse(waitOutput).data.reply.option_id).toBe("small");
  const inbox = cli("inbox", "--json");
  expect(inbox).toHaveLength(1);
  await page.screenshot({ path: "../artifacts/desktop-answer.png", fullPage: true });
  const cookies = await context.cookies();
  const session = cookies.find((c) => c.name === "agentdesk_session");
  expect(session?.httpOnly).toBe(true);
  await stopService();
  // Forced process termination can leave a stale PID; restart must replace it.
  expect(Number(readFileSync(path.join(dir, "server.pid"), "utf8").trim())).toBe(service.pid);
  await startService();
  expect(cli("inbox", "--json")[0].event_id).toBe(inbox[0].event_id);
  cli("claim", "--holder", "e2e-coordinator");
  cli("receipt", inbox[0].event_id, "--holder", "e2e-coordinator");
  expect(cli("inbox")).toHaveLength(0);
  expect(cli("inbox", "--consumer", "auditor")).toHaveLength(1);
  await page.goto(`${endpoint}/?issue=${issue.issue_id}`);
  await expect(
    page.getByText("Coordinator 已接手；後續工作完成與否由專案追蹤。", { exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("手機自由回答、稍後處理、Action 與 Notice Ack", async () => {
  const decision = create("decision", "手機自由回答");
  const action = create("action", "測試待處理事項");
  const notice = create("notice", "測試通知");
  await test.step("mobile login", () => login());
  await test.step("open decision", () =>
    page.getByRole("button", { name: /手機自由回答/ }).click());
  const detail = page.getByRole("region", { name: "議題詳情" });
  await expect(detail.getByRole("heading", { name: "手機自由回答", exact: true })).toBeVisible();
  await test.step("defer decision", () =>
    detail.getByRole("button", { name: "稍後處理", exact: true }).click({ timeout: 5000 }));
  expect(cli("get", decision.issue_id).status).toBe("pending");
  await expect(page.getByRole("button", { name: "移回待回覆" })).toBeVisible();
  await test.step("mobile screenshot", () =>
    page.screenshot({ path: "../artifacts/mobile-question.png", fullPage: true, timeout: 10000 }));
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.getByRole("button", { name: "移回待回覆" }).click();
  await page.getByLabel("你的回答或補充說明").fill("使用我自己的方案，不對應選項。");
  await page.getByRole("button", { name: "送出回答", exact: true }).click();
  expect(cli("get", decision.issue_id).reply.text).toBe("使用我自己的方案，不對應選項。");
  for (const issue of [action, notice]) {
    await page.getByRole("button", { name: "返回清單" }).click();
    await page.getByRole("button", { name: new RegExp(issue.title) }).click();
    await expect(
      page.getByText("「確認收到」只表示你已收到此事項，不代表外部操作已完成。", { exact: true }),
    ).toBeVisible();
    expect(cli("get", issue.issue_id).status).toBe("pending");
    await page.getByLabel("附註（選填）").fill("收到，稍後處理。");
    await page.getByRole("button", { name: "確認收到", exact: true }).click();
    await expect(page.getByRole("heading", { name: "已確認收到", exact: true })).toBeVisible();
    expect(cli("get", issue.issue_id).status).toBe("acked");
  }
  await page.getByRole("combobox", { name: "顯示模式" }).selectOption("dark");
  await expect(page.locator("html")).toHaveClass("dark");
  await page.screenshot({ path: "../artifacts/mobile-dark.png", fullPage: true });
});

test("顯示模式預設跟隨裝置，手動設定持久保存並可恢復跟隨", async () => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto(endpoint);
  const mode = page.getByRole("combobox", { name: "顯示模式" });
  await expect(mode).toHaveValue("system");
  await expect(page.locator("html")).toHaveClass("dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).not.toHaveClass("dark");
  await mode.selectOption("dark");
  await page.reload();
  await expect(mode).toHaveValue("dark");
  await expect(page.locator("html")).toHaveClass("dark");
  await mode.selectOption("light");
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator("html")).not.toHaveClass("dark");
  await page.reload();
  await expect(mode).toHaveValue("light");
  await expect(page.locator("html")).not.toHaveClass("dark");
  await mode.selectOption("system");
  await expect(page.locator("html")).toHaveClass("dark");
  await login();
  await expect(mode).toHaveValue("system");
  await expect(page.locator("html")).toHaveClass("dark");
});

test("過期版本拒收並要求重新確認，登出撤銷 session", async () => {
  const issue = create("decision", "版本衝突測試");
  await login();
  await page.getByRole("button", { name: /版本衝突測試/ }).click();
  await page.getByLabel("你的回答或補充說明").fill("舊版答案不能生效");
  const config = JSON.parse(readFileSync(path.join(dir, "project.json"), "utf8"));
  const update = await fetch(`${endpoint}/api/v1/agent/issues/${issue.issue_id}/update`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${config.agent_token}`,
      "X-Project-ID": config.project_id,
    },
    body: JSON.stringify({
      revision: 1,
      draft: { kind: "decision", title: "新版問題", context: "需求已變動" },
    }),
  });
  expect(update.ok).toBe(true);
  await page.getByRole("button", { name: "送出回答", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "議題版本或狀態已改變。已載入最新內容，請重新確認後再送出。",
  );
  await expect(
    page
      .getByRole("region", { name: "議題詳情" })
      .getByRole("heading", { name: "新版問題", exact: true }),
  ).toBeVisible();
  expect(cli("get", issue.issue_id).reply).toBeUndefined();
  await page.getByLabel("你的回答或補充說明").fill("我已看過新版問題");
  await page.getByRole("button", { name: "送出回答", exact: true }).click();
  await expect(page.getByRole("heading", { name: "已保存你的回答" })).toBeVisible();
  await page.getByRole("button", { name: "登出", exact: true }).click();
  await expect(page.getByRole("heading", { name: "登入收件匣" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "登入收件匣" })).toBeVisible();
});

test("斷線保留未送出回答，恢復連線後可重試", async () => {
  const issue = create("decision", "連線恢復測試");
  await login();
  await page.getByRole("button", { name: /連線恢復測試/ }).click();
  await page.getByLabel("你的回答或補充說明").fill("斷線時保留這段回答");
  await context.setOffline(true);
  await page.getByRole("button", { name: "送出回答", exact: true }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByLabel("你的回答或補充說明")).toHaveValue("斷線時保留這段回答");
  expect(cli("get", issue.issue_id).status).toBe("pending");
  await context.setOffline(false);
  await page.getByRole("button", { name: "重新整理清單", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByText("已連線", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "送出回答", exact: true }).click();
  await expect(page.getByRole("heading", { name: "已保存你的回答" })).toBeVisible();
  expect(cli("get", issue.issue_id).reply.text).toBe("斷線時保留這段回答");
});

test("detached start 自動初始化、在 launcher 結束後存活並重用服務", async () => {
  const detachedDir = mkdtempSync(path.join(tmpdir(), "agentdesk-detached-"));
  let childPID: number | undefined;
  function start(...args: string[]) {
    const output = execFileSync(binary, ["start", "--data-dir", detachedDir, "--json", ...args], {
      encoding: "utf8",
      windowsHide: true,
    });
    return JSON.parse(output).data;
  }
  try {
    const first = start("--listen", "127.0.0.1:0");
    childPID = first.service.pid;
    expect(first.started).toBe(true);
    expect(Number(readFileSync(path.join(detachedDir, "server.pid"), "utf8").trim())).toBe(
      childPID,
    );
    expect((await fetch(first.service.public_url)).ok).toBe(true);
    const reused = start();
    expect(reused.already_running).toBe(true);
    expect(reused.service.pid).toBe(childPID);
    process.kill(childPID!, "SIGKILL");
    childPID = undefined;
    await expect
      .poll(async () => {
        try {
          return (await fetch(first.service.public_url)).ok;
        } catch {
          return false;
        }
      })
      .toBe(false);
    const recovered = start();
    childPID = recovered.service.pid;
    expect(recovered.started).toBe(true);
    expect(childPID).not.toBe(first.service.pid);
    expect((await fetch(recovered.service.public_url)).ok).toBe(true);
  } finally {
    if (childPID) process.kill(childPID, "SIGKILL");
  }
});
