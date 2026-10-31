# 第一版驗證紀錄

日期：2026-10-31。環境：Windows amd64、Go 1.27.2、Vite+ 1.3.0。自動測試使用暫存專案、localhost 與假的 Telegram API；另外依使用者明確授權，使用既有 tg-notify secret 對個人 Telegram 執行實際通知驗證。

## 英文介面與雙語文件

- `README.md` 為英文首頁，`README.zh-TW.md` 保留繁體中文，兩份均有語言切換連結與對應介面截圖。
- `vp -C agentdesk-ui check`：格式、lint、型別檢查通過；正式 Web 與 Windows amd64 單一執行檔建置通過。
- `vp -C agentdesk-ui test`：新增 4 項單元測試，涵蓋瀏覽器語言協商、明確偏好與 fallback，以及英文單複數和繁體中文等待時間格式。
- Playwright headless Chromium：10 項 E2E 通過。原有 6 項繁體中文流程保留，另驗證英文決策回覆、Action Ack、手機版面、草稿與選項在語言切換後保留、偏好重新載入與跨分頁同步，以及登入錯誤即時切換語言。
- 回覆後顯示「下一個待回覆議題」：手機測試驗證中英文按鈕、清單搜尋不阻擋後續議題、略過稍後處理／撤銷／已回答的項目、切換後聚焦標題並捲回頁頂、原始回答持久保存，以及無其他待回覆議題時顯示完成提示與返回收件匣。
- 手機長文字回歸：修正前，390 px 的 mobile viewport 被長標題／路徑撐到 2933 px；修正 grid track、換行與欄位最小寬度後，在 320／375／390／430 px 驗證收件匣、議題詳情、選項與已保存回答均無水平溢位，初始 visual viewport scale 為 1。手機文字輸入、textarea 與 select 的 computed font size 至少為 16 px；這是 mobile Chromium 模擬證據，未宣稱實體 iOS 鍵盤驗證。
- 英文／繁體中文示範截圖均在獨立暫存專案產生；議題內容保留原文。

## 初版已執行

- 公開展示準備：SpiceSharp 五個歷史議題以唯讀方式匯出、移除私人環境資訊，停用 Telegram 在獨立暫存專案重播。1440 px 桌面與 390 px 手機完整頁面截圖已逐張檢查，手機無水平溢位。
- CI 模式：Windows 上實際使用 Playwright headless Chromium，6 項 E2E 全部通過；沒有依賴 CDP 9222。Go vet／測試、Vite+ check、production build 重新通過；前端 unit test 仍無測試檔案。
- `.github/workflows/ci.yml`：actionlint v1.7.12 驗證通過，第三方 actions 使用固定 commit SHA。GitHub runner 結果以 repository 的 Actions 頁面為準。
- [GitHub Actions run 41210787434](https://github.com/Inndy/agentdesk/actions/runs/41210787434)：前端檢查／建置、Windows／Linux／macOS 原生 Go vet／測試／編譯、六種跨平台 binary artifact，以及 Linux headless Chromium 的 6 項 E2E 全部通過。來源 commit 為 `ab4f64a`。
- `CGO_ENABLED=0 go test ./...`：24 項測試通過，包含資料／HTTP／worktree／PID、NIC 選擇、Telegram outbox、一次性登入連結，以及 CLI／啟動設定。
- `go vet ./...`：通過。
- `vp -C agentdesk-ui check`：格式、lint、型別檢查通過。
- `vp -C agentdesk-ui run build`：`vue-tsc` 與正式前端建置通過。
- `vp -C agentdesk-ui test`：沒有前端 unit test 檔案；行為驗證由下列 Playwright E2E 執行，不計為 unit test 覆蓋。
- `make test-e2e`：6 項真實 Chrome CDP E2E 全部通過，包含 detached start、launcher 結束後仍存活、重用現有 server、重啟恢復及 PID 覆寫。每案例獨立 context，不用個人 profile。
- `make build-all JOBS=3`：先建置一次 Web，再平行建置 Windows／Linux／macOS 各 amd64／arm64，6 個 `CGO_ENABLED=0` 目標全部成功。
- `skills/agentdesk/SKILL.md`：skill-creator 的 `quick_validate.py` 驗證通過。
- Claude 與 Codex 的全域 SKILL.md 已安裝，SHA256 與專案來源相同；兩份 runtime.json 指向同一 `~/.local/bin/agentdesk.exe`，全域 executable 與本機測試 binary SHA256 相同，PATH 能解析到該共享 binary。
- 實際 Telegram：修正後測試議題 `N-139854d29fa7` 一次嘗試送出，Bot API 確認 message ID 11450；使用者透過頁面 Ack，附註 `LGTM`。一次性 token 不寫入本報告。服務在 launcher 結束後仍可透過已認證 status API 存取，Telegram 啟用中。

交付 `bin/agentdesk.exe` 與 `bin/agentdesk-windows-amd64.exe` 雜湊相同；E2E 使用前者，資產為目前的 Vue production build。

## 第一版需求對照

| 需求 | 當前證據 |
| --- | --- |
| Go server／CLI、Vue UI、SQLite、單一執行檔 | 實際啟動 binary；瀏覽器從其內嵌資產載入，無獨立 frontend server |
| Decision 選項與自由回答 | E2E 1 選項回覆、E2E 2 手機自由回答；推薦選項未預選，未填答案不能送出 |
| Action／Notice Ack | E2E 2 分別送 Ack 並由 CLI 驗證 acked；頁面明確標示收到不等於外部工作完成 |
| 稍後處理不構成回答 | E2E 2 defer／restore，CLI 確認 pending；store 測試檢查無 reply |
| 冪等 create、回答重送不重複 | E2E 1 透過 CLI 重送同 key；store 測試檢查 changed payload 衝突與事件數 |
| 版本綁定、撤銷與歷史 | E2E 3 舊版本送出拒絕、載入新版要求重新確認；store 測試覆蓋撤銷後不能回答 |
| Durable event、receipt 與 consumer 隔離 | E2E 1 重啟服務後同事件仍存在，receipt 後 coordinator 清空、auditor 仍保留；store 檢查完整歷史 |
| Coordinator lease | CLI 與 store 測試：不同 holder 衝突、錯誤 holder 無法 receipt、過期 lease 可恢復 |
| Wait 逾時不代答、可取消 | CLI 退出碼 3，store 檢查 pending 與空 reply；context cancellation 測試 |
| Watch 斷線退避、sequence 去重 | CLI 真實 HTTP 測試：暫時 503 後重連，重複 seq 不輸出、後續事件保留 |
| 人類／agent 權限分離 | HTTP 測試：agent token 不能登入或回覆，不能讀另一專案；actor 由 server 決定，拒絕偽造欄位 |
| Token 登入與 Web session | E2E 1 fragment 立即清除、cookie HttpOnly；HTTP 測試 rotation／logout 撤銷、Origin 防護 |
| Telegram 通知與一次性登入連結 | 假 API 測試單次送出、queue 去重、429 退避、unknown 不自動重送、重啟保存及跳過已處理議題；HTTP 測試 token 到期／撤銷／並行兌換／重開已登入連結，另有真實 Telegram 與使用者 LGTM |
| Detached bootstrap 與 Makefile | 真實 binary 的 start 可初始化、驗證重用、保存設定及在 launcher 結束後持續執行；Make check/test/build-all 已實際執行 |
| Worktree 共用專案 | 測試建立暫存 Git repo 與 linked worktree，兩者探索到相同資料目錄 |
| PID 檔生命週期 | Go 測試檢查正常關閉／啟動失敗清理、stale PID 覆寫及鎖不提前釋放；實際程序重啟 E2E 檢查新 PID |
| 手機／桌面可操作、斷線恢復 | 1440×1000 桌面與 390×844 手機尺寸的實際 Chrome；手機無水平溢出；E2E 4 斷線保留草稿、重連清除錯誤後可再送 |
| 主題跟隨裝置與持久化 | 專用 E2E 模擬裝置深／淺色變更，驗證預設跟隨、手動覆寫、重新整理保存、恢復跟隨與登入後保留 |
| Claude Code／Codex 基本整合 | 共用 shell skill 與兩平台安裝路徑已提供，CLI 工作循環已實測；不依賴平台注入 API |
| 既有 ADR／記憶並存 | 服務只保存議題與引用；skill 指示由 coordinator 依專案規則更新，無跨專案自動修改 |

Desktop 另一個對話的獨立 built-in Browser 檢查也已回報登入、推薦未預選、defer／restore、自由回答與 Action／Notice Ack 成功；它驗證的是較早的同一版 binary，報告位於 `artifacts/browser-assist.md`，不取代上述最新 binary 的 E2E。

## 尚未執行的環境驗證

- 實體手機、使用者自己的 VPN、HTTPS 反向代理與防火牆。
- Linux／macOS 與 arm64 的實體桌面操作、OS 開啟瀏覽器與 VPN；GitHub hosted runner 的原生 Go 測試不等同這些情境。
- 以使用者的預設瀏覽器實際啟動 `login --open`；瀏覽器登入流程已由隔離測試驗證，OS 開啟命令由程式依平台提供。
- Claude Code／Codex 新一輪的 skill 載入與 agent 實際選用的背景 job；安裝與共享 executable 已核對，不宣稱已替兩個宿主啟用 `/loop`、goal 或即時注入。

Hub、Telegram 內回答、進階提醒、多使用者、多主同步與 Channels／app-server adapter 均屬後續範圍。
