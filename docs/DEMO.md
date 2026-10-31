# SpiceSharp 示範

README 截圖來自 SpiceSharp 的五個真實歷史議題：記憶體回收、Launcher 技術選型、serial console 終端大小同步、pinned CA 政策，以及 status overlay 操作請求。

這是歷史內容重播。示範將議題重新建立為待回覆狀態，不保留原始回答、原始狀態或原始時間，也不代表相關技術提案已定案。每個議題的背景均附重播標示。

## 資料

`examples/spicesharp-demo.json` 僅包含建立議題所需的內容欄位，沒有登入 token、session、Telegram 設定或人類回答。私人檔案路徑與 IP 已替換。

需要重新從來源擷取時，可使用唯讀匯出工具：

```sh
python scripts/export-spicesharp-demo.py --source-db /path/to/SpiceSharp/.git/agentdesk/agentdesk.db
```

工具僅選取腳本中列出的五個歷史議題；發布新的資料前，仍應檢查正文是否含有不宜公開的內容。重建截圖只需 repository 內的示範 JSON，不需要來源資料庫。

## 重建截圖

先建置 Agentdesk，並啟動獨立的 CDP Chrome（預設 port 9222）：

```sh
vp node scripts/build.mjs
vp -C agentdesk-ui run demo:screenshot
```

可用 `AGENTDESK_CDP_URL` 指定 CDP 端點，或用 `AGENTDESK_BINARY` 指定另一個執行檔。設定 `CI=true` 且不提供 CDP 端點時，使用 Playwright 自己啟動的 headless Chromium；先執行 `vp -C agentdesk-ui exec playwright install chromium` 安裝瀏覽器。

預設產生繁體中文介面截圖。設定 `AGENTDESK_DEMO_LOCALE=en` 後執行同一指令，會產生英文介面版本，檔名加上 `-en`；議題正文仍保留來源的原文。

腳本建立獨立的系統暫存資料目錄，服務只監聽隨機 localhost port，明確停用 Telegram。登入資訊只在記憶體中使用，網址 token 清除後才截圖，不錄製 trace 或影片；完成後關閉腳本自己的瀏覽器 context 與服務。暫存目錄保留在 OS 暫存區，不會寫入正式專案資料。

輸出：

- `docs/images/spicesharp-desktop.png`：1440 px 桌面版決策問題。
- `docs/images/spicesharp-mobile.png`：390 px 手機版操作請求，包含無水平溢位檢查。

兩張皆為完整頁面截圖，使用深色裝置偏好，未送出示範回答。
