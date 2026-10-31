# Agentdesk

本機的人類議題收件匣。Go 單一執行檔同時提供 CLI、SQLite 持久化服務與 Vue Web UI。第一版包含 Decision、Action、Notice、自由回答／選項、Ack、稍後處理、版本衝突檢查、事件補讀與 coordinator 接手；Hub、Telegram 與即時 session 注入延後。

## 建置

需要 Go 1.25 以上、Vite+（`vp`）與專案指定的 pnpm。Vue 與目前 `vue-tsc` 使用相容的 TypeScript 5.9。

```sh
vp -C agentdesk-ui install
vp node scripts/build.mjs
```

輸出為 Windows 的 `bin/agentdesk.exe`，或 Linux/macOS 的 `bin/agentdesk`。`vp node scripts/build.mjs --all` 建置六個目標：Windows／Linux／macOS 各 amd64、arm64。所有 Go 建置設定 `CGO_ENABLED=0`；Web 資產已內嵌，不需在執行時安裝 Node、SQLite 或攜帶 dist。

若手動建置，先執行 `vp -C agentdesk-ui run build`，再執行 `go build -o bin/agentdesk ./cmd/agentdesk`（Windows 加 `.exe`，並將環境變數 `CGO_ENABLED` 設為 `0`）。

## 開始使用

以下假設 `agentdesk` 已加入 PATH；也可直接使用完整執行檔路徑。

```sh
agentdesk init --name my-project
agentdesk serve
```

在另一個終端以人類身分登入：

```sh
agentdesk login --open
```

服務也接受 `serve --open`。`login` 不帶 `--open` 會輸出可複製的登入網址；網址具有登入能力，請只自行使用。`login --rotate` 產生新 token 並撤銷所有現有 Web session。Token 有效 30 天，登入 session 有效 7 天；token 不會出現在一般啟動日誌、agent 指令輸出或 HTTP 存取日誌。

Agent 建立問題：

```sh
agentdesk create --file examples/decision.json --key import-first-batch-v1 --json
agentdesk list --pending --json
agentdesk get D-RETURNED-ID --json
```

複雜內容也可從 stdin 透過 `--file -` 傳入。Web 由人類選擇方案或自由回答。推薦方案不預選，逾時不會代答。Action／Notice 的「確認收到」只代表 Ack，不表示外部工作完成。

Coordinator 的接手流程：

```sh
agentdesk claim --holder session-unique-id --ttl 60s --json
agentdesk inbox --consumer coordinator --json
# 將事件與回答納入專案持久工作紀錄後，才送 receipt：
agentdesk receipt E-RETURNED-ID --consumer coordinator --holder session-unique-id
agentdesk wait --consumer coordinator --timeout 50s --json
agentdesk watch --consumer coordinator --jsonl
```

每次 session 使用唯一 holder，持續工作時在 60 秒內以相同 claim 更新租約。另一個 holder 取得相同 consumer 會收到衝突。租約過期後可恢復接手；副作用工作仍需以穩定工作 ID 和持久紀錄防止重複。Receipt 逐事件保存，不會確認其他尚未處理的事件，也不影響其他 consumer。

`wait ISSUE-ID` 等該議題離開 pending；不帶 ID 等 consumer 的事件。`watch` 以 long polling 阻塞、斷線退避，輸出 JSONL；同一程序重連依 seq 去重，重新啟動時重播未 receipt 的事件。可用 `--after SEQ` 指定補讀起點；只有在已持久保存該進度時才使用。

`history ISSUE-ID` 檢視歷史與 receipt。`update ISSUE-ID --revision N --file question.json` 更新尚未回答的議題並增加 revision；`withdraw ISSUE-ID --revision N` 撤銷。已回答的問題不可修改／撤回答案，需建立關聯的新問題。

## 手機與 VPN

VPN 由使用者管理。預設僅監聽 loopback；要從手機連線，明確指定 VPN 介面的位址及手機可達 URL，例如：

```sh
agentdesk serve --listen 100.100.10.20:7331 --public-url http://100.100.10.20:7331
agentdesk login
```

將輸出的網址複製到已連上 VPN 的手機。`--listen 0.0.0.0:7331` 也需要明確的 `--public-url`；它會監聽所有介面，請自行限制防火牆。第一版 HTTP 適用於受保護 VPN；HTTPS 可透過既有反向代理，將 `--public-url` 設為 HTTPS origin。服務本身不終結 TLS。手機不能使用 localhost URL。

Web 先從 URL fragment 取得 token，立即清除網址中的 token，再交換為 HttpOnly、SameSite=Strict cookie。HTTPS public URL 使用 Secure cookie。人類回覆要求同源 Origin 與 JSON；agent 的 Bearer token 不可登入 Web 或提交回答。

頁面預設跟隨裝置的深淺色偏好，裝置設定變更時會同步更新。右上角可改選深色或淺色，也可切回「跟隨裝置」；選擇會在該瀏覽器保存，重新整理與登入後仍保留。

## 專案與資料

在目標專案執行 `init`。Git 專案儲存於 Git common directory 下的 `agentdesk`，所以 linked worktree 共用同一 project ID、憑證與資料庫；非 Git 專案使用 `.agentdesk`，子目錄會向上探索已初始化的專案。可用 `--data-dir PATH` 或 `AGENTDESK_DATA_DIR` 指定其他位置。

`project.json` 保存 project ID、服務端點與 agent token；`human.json` 獨立保存人類 token；`agentdesk.db` 保存議題、歷史、session 雜湊、receipt 與 lease。這些是本機私有資料，不應提交到版本控制。Unix 目錄／憑證使用 0700／0600；Windows 繼承目錄 ACL。相同 OS 使用者仍能存取本機檔案，本產品的 agent/human API 權限分離不構成 OS 層的隔離。

同一資料目錄只允許一個 server；停止程序後 OS 會釋放鎖。SQLite 使用 WAL，請將資料放在本機磁碟。備份時停止服務後複製整個資料目錄。每個專案預設一個 service／port；其他專案使用不同 `--listen` port。

## Agent 安裝

將本專案的 `skills/agentdesk` 整個資料夾複製到目標專案：

- Claude Code：`.claude/skills/agentdesk/SKILL.md`。
- Codex：`.agents/skills/agentdesk/SKILL.md`。

將 binary 加入 PATH，或在專案 agent 指示中提供路徑。已有的 `/loop` 或 active goal 可使用此 skill 的收件迴圈；安裝不自動啟動排程／goal，不修改 sandbox、approval 或既有 ADR／記憶規則。首版使用 shell CLI，不依賴 Channels 或 app-server。實際宿主載入狀態請由各自 session 確認。

## 驗證

```sh
go test ./...
vp -C agentdesk-ui check
vp -C agentdesk-ui run build
vp -C agentdesk-ui test --passWithNoTests
```

端到端測試使用已建置的 binary 與獨立 Chrome CDP。預設 `http://127.0.0.1:9222`，可用 `AGENTDESK_CDP_URL` 指定；建立獨立測試 context，不使用個人 profile。先啟動隔離的 CDP Chrome，再執行：

```sh
vp -C agentdesk-ui run test:e2e
```

測試僅操作暫存專案／localhost，涵蓋登入、選項／自由回答、Ack、手機版面、版本衝突、CLI wait、receipt 與服務重啟。截圖位於 `artifacts`。不錄製帶 token 的 trace、影片或登入網址。

CLI stdout 為版本化 JSON（`login` 特例輸出秘密網址），診斷在 stderr。Exit code：0 成功、1 執行錯誤、2 參數錯誤、3 wait 逾時、4 revision／lease 衝突。API 詳見 [docs/API.md](docs/API.md)。

目前已執行的驗證及實機限制見 [docs/VALIDATION.md](docs/VALIDATION.md)。
