# Agentdesk 規格

狀態：初版草案，供實作與後續決議使用。
日期：2026-10-31

本文件彙整已確認的需求，以及討論中提出的設計建議。標示「建議」或「待決議」的內容，不代表人類已批准該技術選擇。本文件本身不啟用任何 agent goal、排程、Telegram bot 或遠端服務。

## 1. 目的與使用情境

Agentdesk 是獨立、可跨專案重用的人類議題收件匣，供 AI agent 提出需要人類決議、處理或確認收到的事項。

主要情境：主要 coordinator agent 持續協調 sub-agent 或外部 agent，非同步推進多項開發工作。人類不一定在電腦前；agent 可以先保存問題並繼續其他工作，人類稍後透過手機或桌面介面回答，coordinator 再接續相關工作。

Agentdesk 必須與各專案既有的 ADR、計畫與記憶系統並存。

## 2. 已確認的需求與界線

- 使用 Go，提供跨平台的單一 binary。
- 目標平台為 Windows、Linux、macOS；CPU 架構矩陣於實作時訂定。
- 第一版交付本機 CLI 與 Web UI；後續已確認加入 Telegram 單向通知與議題登入連結，中央 Hub 與 Telegram 內回答仍延後。
- 第一版由使用者自行提供 VPN 內網連線，Web 採 token-based 登入，支援本機啟動瀏覽器帶入 token，以及複製登入網址到手機使用。
- 同一 binary 可啟動 server，也可作為 agent 使用的 CLI。
- 先支援 Claude Code 與 Codex；核心介面須能供其他具 shell 能力的 agent 使用。
- 同時支援單機與常駐主機部署，以單機為預設。
- 設計前提是主要 coordinator 預期持續運作，並配合 agent 的 `/loop`、`/goal` 或等效工作循環。
- 各專案擁有自己的資料與記憶。
- 需要能共用同一個 Telegram bot 的中央路由設計；是否啟用 Telegram、是否首版提供完整互動，仍待決議。
- 不把特定 agent 的即時訊息注入能力作為核心流程成立的必要條件。
- Agentdesk 不負責重建完整的開發協調器、替換 agent 記憶系統，或自動擴大 agent 的執行權限。

## 3. 議題類型

### 3.1 Decision：需要人類決議

Agent 提供：

- 明確的問題與必要背景。
- 若有適合的方案，提供多個具穩定 ID 的選項及利弊。
- 可推薦一個選項，並說明理由。
- 影響範圍、相關工作與文件連結。

人類可以選擇選項，或直接輸入自己的回答。純自由回答的問題也必須支援。

推薦選項不得因為預選、等待時間經過或 CLI 逾時而自動成立。回答須綁定人類實際看到的議題版本。

### 3.2 Action：需要人類處理

Agent 提供具體事項、原因與可操作的步驟。基本互動為 Ack，可附註。

Ack 的預設含義是「已確認收到」，不得自動推論為外部操作已完成。若工作依賴實際完成確認，需明確描述所需回覆；是否另設「已完成」狀態與按鈕，列為待決議。

### 3.3 Notice：單方向告知

Agent 提供需要人類知道的內容。人類按 Ack，可附註。

通知送出、訊息已讀與人類 Ack 是不同狀態。只有明確操作才能形成 Ack。

### 3.4 共通行為

- 可稍後處理，不等於回答、拒絕或 Ack。
- 可撤銷尚未完成的議題，保留歷史。
- 需要人類回覆的議題不得由 agent 使用 agent 憑證代答或代 Ack。
- 涉及特定行為的回答只適用於該議題描述的範圍，不構成其他工作或未來行為的概括授權。

## 4. 系統角色與資料所有權

### 4.1 Project service

每個專案具有自己的權威狀態與 project service。它負責：

- 議題全文、選項、版本、回答、Ack 與歷史。
- Coordinator 與其他 consumer 的事件消費進度。
- 待接手的回答、受阻擋工作與相關文件引用。
- 驗證人類回答對應的議題版本與狀態。
- 提供專案 CLI/API 與事件等待能力。

同一專案的多個 worktree 共用同一個專案身分與 project service；branch、worktree、task 是來源資訊，不應因目前工作目錄不同而自動建立另一份資料。

資料實際存放目錄、專案身分初始化與 worktree 探索規則，待決議。

### 4.2 Hub

共用 Telegram bot 時，Hub 是中央通訊入口，負責：

- 唯一的 Telegram update 接收流程。
- Bot 憑證、人類身分綁定、專案註冊與路由。
- 發送通知、處理按鈕與自由文字回覆。
- 持久保存尚未成功轉交的傳輸訊息。
- 提供跨專案 Web 收件匣入口。

Hub 不保存或管理各專案的完整 agent 記憶。Hub 的資料庫保存路由與傳輸狀態，不取代各專案的權威資料庫。

為通知與列表所需的摘要，可按明確規則暫存；保存範圍、保留時間與清理策略待決議。

### 4.3 Coordinator 與 sub-agent

- 每個專案的 coordinator 是回答的預設接手者。
- Sub-agent 可以提出議題，建立者不必是後續接手者。
- Sub-agent 結束或 session 更換，不得使議題與回答消失。
- Coordinator 負責解讀回答、更新專案計畫與記憶、安排後續工作、回報處理結果。
- Coordinator 意外中斷後，新的 session 可從專案狀態恢復尚未接手的回答。
- Coordinator 替換或短暫重疊時，須避免同一工作被兩個 coordinator 同時啟動；具體 claim/lease 機制待決議。

## 5. 部署模型

### 5.1 預設單機

同一程序可執行 project service、內嵌 Hub 與 Web UI。CLI 連線本機服務。

手機透過私人網路或受保護的 HTTPS 入口存取。主機休眠或服務停止時，不宣稱 Web 仍可收件或 agent 仍會執行。

第一版由使用者自行設定 VPN，不內建 VPN 或公開入口管理。預設仍只監聽 loopback；透過明確設定監聽位址及對外基底 URL，供 VPN 內的手機存取。產生供手機使用的登入網址時須使用該基底 URL，不能使用 localhost。

### 5.2 中央 Hub 與多個專案

Hub 執行於常駐主機；各專案 service 可以位於不同開發電腦或主機。

建議由 project service 主動建立至 Hub 的長連線。Hub 透過該連線路由請求與回覆，各專案無須開放公開入站 port。實際傳輸協定待決議。

### 5.3 實作與封裝建議

- Go binary 內嵌 Web 靜態資產，例如使用 `go:embed`。
- 各專案使用 SQLite 保存狀態；後續 Hub 使用獨立資料庫。
- SQLite 採不依賴 CGO 的 driver；實作預設選用 `modernc.org/sqlite`，以 `CGO_ENABLED=0` 驗證建置。
- 相同 binary 透過子命令或模式啟動各角色。
- 不在第一版實作多主資料庫同步或自動選主。
- 目標作業系統為 Windows、Linux、macOS；具體 CPU 架構與最低系統版本尚待訂定。

## 6. Coordinator 工作循環

每輪建議依序執行：

1. 讀取尚未接手的回答、Ack 與其他相關事件。
2. 檢查議題版本與目前工作是否仍適用。
3. 將回答納入專案計畫、記憶或待執行工作。
4. 回報已接手；這不等於工作已完成。
5. 恢復受該決議阻擋的工作，協調 sub-agent，推進其他已授權工作。
6. 保存新問題與通知。
7. 若無可執行工作，阻塞等待事件，或交由宿主排程下一輪。

問題須標示其阻擋範圍。單一問題不應使所有互不依賴的工作停下。

`/loop` 適合週期性執行收件與協調；`/goal` 適合在目標執行過程納入收件與恢復工作。不得為了保持運作而自動創造新目標、重啟已被人類停止的 goal，或規避宿主的預算與權限限制。

若使用者已取消、暫停或目標已完成，回答到達不能直接使 agent 擴大或恢復原本已停止的工作；須保留回答，依宿主與既有授權規則處理。

## 7. CLI 草案

以下是概念性介面，實作前可調整名稱，但必須保留對應能力。

```sh
# 單機：project service、Web UI 與內嵌 Hub
agentdesk serve --with-hub

# 中央通訊入口
agentdesk hub serve

# 專案 service 主動連線中央 Hub
agentdesk serve --hub https://hub.example.com

# 建立議題；複雜內容從檔案或 stdin 傳入
agentdesk create --file question.json --json

# 查詢與恢復
agentdesk get D-123 --json
agentdesk list --pending --json

# 每輪讀取尚未接手的事件
agentdesk inbox --consumer coordinator --json

# 無可執行工作時等待事件
agentdesk wait --consumer coordinator --timeout 50s --json

# 等待特定議題
agentdesk wait D-123 --timeout 50s --json

# 長期訂閱事件
agentdesk watch --consumer coordinator --jsonl

# 已把回答納入專案狀態後，明確確認接手
agentdesk receipt EVENT-ID --consumer coordinator
```

介面要求：

- 機器輸出採版本化 JSON；串流輸出採 JSONL。
- 機器輸出在 stdout，診斷在 stderr。
- 建立議題返回穩定 ID、revision，以及可用時的 Web URL。
- `create` 支援冪等鍵，重試不產生重複議題。
- `inbox` 讀取不刪除，不因列出事件而推進已接手進度。
- `wait` 可在事件已存在時立即返回；逾時不修改議題、不代選答案。
- `watch` 支援中斷後補讀及去重，重連採退避。
- 事件等待使用阻塞式機制，例如 SSE 或 long polling；不要求模型頻繁輪詢。
- `receipt` 是 agent 接手事件的回執，與人類 Ack 分開。
- 中斷等待須可安全取消，且不丟失尚未接手的事件。
- 精確的 exit code、API schema、事件確認粒度與進度保存方式，待決議。

## 8. 議題資料模型草案

最少包含：

- `project_id`：穩定的專案識別。
- `issue_id`、`revision`、`kind`、`status`。
- `title`、`context`。
- `options`：選項 ID、標籤、說明與利弊；可省略。
- `recommended_option`、`recommendation_reason`；可省略。
- `created_by`、`owner_coordinator`、`origin_session`、`task_ref`。
- `blocking_scope`、相關 branch/commit 參考與 `related_documents`。
- `created_at`、`updated_at`。
- 回答或 Ack：內容、選項 ID、對應 revision、已驗證的人類身分與時間。
- 修改、撤銷、交付與接手歷史。

自由回答不應被強制映射成最接近的選項。Agent 若無法明確理解回答，可建立具關聯的新問題。

## 9. 狀態與交付語義

議題狀態、通訊交付、agent 接手及後續工作完成，必須分開記錄。

### 9.1 人類回覆

- Decision：待回答 → 已回答，或撤銷。
- Action/Notice：待 Ack → 已 Ack，或撤銷。
- 稍後處理是顯示或提醒屬性，不代表完成。

### 9.2 經 Hub 回覆

```text
Hub 驗證回覆者並持久收件
  → 等待專案接收
  → 專案驗證議題版本與目前狀態
  → 專案持久寫入，回答／Ack 正式成立
  → coordinator 讀取
  → coordinator 回報已接手
  → 後續工作完成，若該議題需要追蹤
```

Hub 尚未取得專案確認時，UI 顯示「已收件，等待專案確認」。不得顯示決議已生效或 agent 已處理。

若專案回報版本過期、議題撤銷或已有衝突回答，保留原始回覆與拒收原因，向人類要求重新確認，不靜默套用到新版問題。

### 9.3 可靠性

- 採至少一次交付，允許重送；以穩定事件 ID 去重。
- 事件與對應的狀態變更須原子保存，避免有回答卻沒有可補讀事件。
- 傳輸佇列只有取得下一個持久保存端的確認後，才能標記已轉交。
- Hub 轉交完成不等於 coordinator 已接手；專案須保留未接手狀態。
- 暫時離線、程序重啟與 session 更換後仍能恢復。
- Agent 回執不得造成其他獨立 consumer 的事件被刪除。
- 不宣稱跨資料庫、agent 或外部工具具有 exactly-once 執行保證。
- 副作用工作需利用穩定工作 ID、持久工作紀錄與可重入設計避免重複執行。

## 10. Web UI

沿用 `agentdesk-ui` 的 Vite+ 骨架，使用 Vue、TypeScript、shadcn-vue 與 Tailwind CSS；建置產物透過 `go:embed` 嵌入 Go binary。

建議提供手機與桌面皆可用的響應式 Web UI，靜態資產內嵌 binary。

第一版建議畫面：

- 跨專案待處理清單：類型、優先度、等待時間、阻擋範圍與連線狀態。
- 議題詳情：背景、選項、推薦理由、相關工作與文件。
- 選項回答、自由回答、Ack、附註與稍後處理。
- 歷史紀錄及實際交付狀態。

專案離線時，僅展示允許暫存且可取得的摘要，並標示狀態。手機頁面送出過期 revision 時須顯示衝突並重新取得問題。

是否需要 PWA、瀏覽器推播或離線頁面，待決議；不可把這些當作手機收件必備條件。

## 11. Telegram 整合

目前已交付的第一階段是 project service 直接發送單向通知：新議題一則訊息，包含議題連結及獨立的一次性登入 token；只呼叫 sendMessage，不接收 bot update。服務可引用既有 tg-notify secret，不與既有 update 接收端競爭。通知採 SQLite outbox，已送出不重送，明確限流／暫時拒絕退避重試，不確定是否已送達時保留 unknown 以避免重複。以下中央接收與互動規則供後續階段使用。

- 同一個 bot 只有一個受控的 update 接收流程，由 Hub 或既有接收端負責。
- 若 bot 已供其他系統使用，須由既有接收端分流或整合 Hub，不啟動競爭收件者。
- 專案 service 與 agent session 不直接各自接收同一 bot 的 update。
- 訊息與 callback 必須綁定 `project_id + issue_id + revision`，不能依模型猜測或「最後一個問題」路由。
- 自由文字回答應使用訊息引用或明確選定的問題；無法唯一識別時要求選擇。
- 驗證 Telegram 使用者身分與專案存取權限；不能只信任 callback 內傳入的使用者欄位。
- 通知建議先提供摘要與 Web 連結，之後可加入 Ack、簡單選項與自由回答。
- Telegram 不是權威議題保存系統；Hub 的傳輸佇列與專案資料庫必須各自持久保存。
- 摘要包含多少專案內容、是否接受機密內容進入 Telegram、刪除與保留策略，待決議。

## 12. Claude Code 與 Codex 整合

### 12.1 共通 skill

Skill 教導 agent 如何分類議題、撰寫背景與選項、描述阻擋範圍、查詢與等待回答、回報接手，以及何時更新專案文件。

共通工作流程不依賴單一平台的內部 session 儲存格式。平台安裝方式與 hooks 設定由各自 adapter 處理。

### 12.2 Claude Code

- `/loop` 可執行收件與協調 skill；可考慮專案 `.claude/loop.md` 整合。
- 排程僅在符合宿主生命週期的情況下執行；不可假設無限存續或每次都準時。
- Channels 可選擇性推送事件到已啟用的 session，但其 transport 成功不是 agent 已處理的回執。
- 自訂 Channels 的 research preview、載入政策與版本限制須於安裝時檢查。

### 12.3 Codex

- `/goal` 的目標執行流程可納入讀取回答與恢復工作。
- 可選 app-server adapter 對已明確綁定、可存取的 thread 使用 `turn/start` 或 `turn/steer`。
- `turn/steer` 必須核對目前 active turn，處理回合切換競態。
- 背景 hook 的完成不保證啟動新回合；hooks 適合作為補讀與恢復入口。
- 不假設能控制任意已開啟的 Codex CLI 或桌面 session。

平台能力來自文件研究，尚未進行實際安裝版本與 session 的整合驗證。Skill 與 adapter 必須明確報告目前可用的能力與退回 CLI 的方式。

## 13. 與 ADR、計畫及記憶並存

- 各專案保留其既有 ADR、計畫與 agent 記憶格式。
- Agentdesk 保存議題與回覆，不自動收集整段 session、完整記憶或整個 repository。
- 議題可引用既有 ADR、文件、issue、commit 或工作項目。
- 需要成為架構紀錄的回答，由 coordinator 依專案規則在正確分支更新 ADR，引用議題 ID、回答時間與來源。
- 一般通知、一次性操作或例行確認不必產生 ADR。
- 不由 Hub 自動跨專案修改 ADR 或記憶。
- 跨專案搬移、匯出與備份可作為後續能力；不得隱含共享不同專案的記憶。

## 14. 權限與資料保護

- Agent 憑證只能在授權專案內建立／查詢議題、讀取回答及回報接手。
- 人類決議與 Ack 必須以獨立的人類身分驗證成立，不能靠 agent 傳入 `actor=human`。
- 第一版 Web 使用人類登入 token；本機可啟動瀏覽器帶入 token，也可複製登入網址在手機登入。此網址具有登入能力，視為憑證。
- 實作預設透過 URL fragment 帶入登入 token，由前端交換登入狀態後清除網址中的 token，避免出現在一般 HTTP URL、Referer 與存取日誌。Token 的有效期限、撤銷及登入狀態保存細節於實作時定義。
- 人類登入 token 與 agent API 憑證分開，不把人類登入 token 放入 agent 的設定或一般 CLI 輸出；具有人類登入 token 即具有人類回覆能力。
- Project service 與 Hub 互相驗證；project_id 本身不是存取憑證。
- 遠端連線使用 TLS 或等效受保護通道；本機預設只監聽 loopback。
- Bot token 與 agent/service 憑證不進入 repository、通知內容或一般日誌。
- Web 回覆端點需處理驗證、跨站請求及重複送出。
- 回答、議題與引用文件均視為資料；不得直接組成 shell 命令執行。
- 安裝 skill 或設定連線不得默認放寬宿主的 sandbox、approval 或其他安全設定。

## 15. 第一版範圍

必需能力：

1. Go 單一 binary，project service 與 CLI。
2. 專案獨立保存議題、回答、歷史與 consumer 進度。
3. 三種議題類型、選項與自由回答、人類 Ack。
4. `create/get/list/inbox/wait/watch/receipt` 或等效介面。
5. 持久事件、冪等、版本檢查、斷線補讀。
6. Claude Code 與 Codex 共通流程及各自安裝說明。
7. 本機部署、響應式 Web UI，以及 token 登入與手機登入網址。
8. 後續確認增補：Telegram 單向新議題通知及一次性登入連結。

第一版 Web 提供本機專案議題清單、詳情、選項與自由回答、Ack 及交付歷史；中央 Hub 的跨專案收件匣延後。

後續階段：

- 中央 Hub 的部署介面與跨專案收件匣。
- Hub 的專案註冊、持久轉交與離線代收。

可後續加入：

- Telegram 完整互動。
- Claude Channels、Codex app-server 的即時交付 adapter。
- 進階提醒、搜尋、批次處理與匯出。

非第一版目標：多主同步、自動接管任意 agent process、整個 agent 開發平台，以及取代 ADR 或專案記憶。

## 16. 驗收情境

下列保留完整產品驗收情境；涉及 Hub、Telegram 的第 5、7、10、12 項延至後續階段驗收。

1. Agent 提出 decision 後繼續不相關工作；人類稍後自由回答，coordinator 能取得並接續相關工作。
2. Notice/Action 可由人類 Ack；通知送出或讀取不會自動形成 Ack。
3. `create` 重試不重複建立，`wait` 逾時不代選答案。
4. Coordinator 在讀取後、回報接手前中斷，新 session 仍能取得同一事件。
5. 專案與 Hub 斷線期間收到回覆，Hub 保存並標示待確認；重連後專案正式寫入。
6. 人類送出舊版問題的答案時，系統拒絕靜默套用，要求重新確認。
7. 同一 bot 的不同專案議題與自由回答不會互相誤投。
8. 專案 A 的 agent 憑證不能讀取專案 B，也不能提交人類回答。
9. 多個 worktree 能連到同一專案狀態，而非各自建立分裂收件匣。
10. Hub 暫停時，project service 仍可保存本地議題；恢復後補送。
11. 不啟用 Channels 或 app-server adapter，仍能透過持續運作的 coordinator 與 CLI 完成流程。
12. 專案記憶與 ADR 留在專案內，Hub 不需取得完整內容即可路由通知與回覆。
13. 本機可啟動瀏覽器帶入 token；使用 VPN 可達位址產生的登入網址，可複製到手機登入並回答議題。
14. Agent 憑證不能用於 Web 人類登入；登入網址中的 token 不出現在一般伺服器存取日誌。
15. Windows、Linux、macOS 的選定架構可在 `CGO_ENABLED=0` 下建置；各平台執行驗證結果須與交叉編譯結果分開記錄。

## 17. 待決議清單

- 三平台的最低系統版本；第一版架構矩陣為各平台 amd64／arm64。
- 專案身分、資料目錄、worktree 與 CLI endpoint 的第一版實作預設見第 19 節。
- Hub 與 project service 傳輸協定及配對方式。
- Web 人類登入 token 的有效期限、撤銷與登入狀態保存，以及 service 與 agent 憑證管理細節。
- Action 是否需要獨立「已完成」確認。
- Coordinator claim/lease 與回執進度模型。
- 議題實質修改後的重新回答規則，以及回答能否撤回／修正。
- Hub 摘要暫存、傳輸佇列、歷史與備份的保留策略。
- 提醒、優先度、到期日、稍後處理的具體語義。
- API、事件 schema 與 exit code 的版本相容策略。

## 18. 研究來源

以下是 2026-10-31 設計研究使用的官方文件；平台能力需對安裝版本再次驗證。

- Claude Code 排程與 `/loop`：https://code.claude.com/docs/en/scheduled-tasks
- Claude Code Channels：https://code.claude.com/docs/en/channels
- Claude Code Channels contract：https://code.claude.com/docs/en/channels-reference
- Codex 長時間工作：https://learn.chatgpt.com/docs/long-running-work
- Codex app-server：https://learn.chatgpt.com/docs/app-server
- Codex hooks：https://learn.chatgpt.com/docs/hooks
- Telegram Bot API：https://core.telegram.org/bots/api

## 19. 第一版實作預設

以下記錄第一版及後續已確認增補的實作預設；操作與 API 細節見 `README.md`、`docs/API.md`。

- Go 1.25 以上，`modernc.org/sqlite` v1.59.0，SQLite WAL；六個平台／架構目標均以 `CGO_ENABLED=0` 建置。
- 使用 `init` 明確初始化 project ID。Git worktree 從 Git common directory 探索共用狀態；非 Git 專案使用 `.agentdesk` 並支援向上探索。可透過 `--data-dir` 或 `AGENTDESK_DATA_DIR` 覆寫。
- 同一資料目錄只允許一個 server，以 `server.lock` 的作業系統檔案鎖避免重複啟動。執行時寫入 `server.pid`（十進位 PID），正常關閉或啟動失敗時清除；當程序強制終止留下舊檔，下一次取得鎖的 server 會覆寫它，不以 PID 是否存在判斷服務擁有權。CLI 從專案設定取得實際 endpoint；不同專案使用獨立 token、資料庫及 port。
- 第一版單一人類 owner。人類 token 與 agent token 分檔，登入 token 有效 30 天，Web session 有效 7 天；`login --rotate` 可撤銷 token 與全部 session。這是 API 角色區分，不是相同 OS 使用者之間的檔案隔離。
- `serve --open`／`login --open` 可開啟本機預設瀏覽器；`login` 可輸出供手機使用的秘密網址。VPN 位址透過 `--listen` 與 `--public-url` 明確設定；HTTP 僅供受保護的本機／VPN 網路，HTTPS 使用既有反向代理。
- Consumer 的 receipt 逐事件保存。Coordinator 以唯一 session holder 取得／更新 lease，預設 60 秒；其他 holder 在到期前不得接管。外部副作用仍需專案持久工作紀錄與冪等設計。
- 事件以 long polling 等待；watch 使用 sequence 補讀及斷線退避，不自動 receipt。新程序重播未 receipt 的事件。
- 實質改題限 pending 狀態並增加 revision；舊版回覆回傳衝突。已回答議題不能改答案或改題，需建立關聯的新問題。稍後處理為獨立布林標記；Action Ack 仍僅代表收到。
- 本機 API 與機器輸出採 schema_version 1；exit code 為 0 成功、1 執行錯誤、2 參數錯誤、3 等待逾時、4 版本／lease 衝突。
- `start` 可自動初始化並 detached 啟動／驗證重用專案服務；`status` 透過已認證 API 回報 PID 與 public URL，不依賴 PID 檔判定身分。啟動選項及 Telegram 設定檔路徑可持久保存。
- 啟用 Telegram 且未指定 listener 時，以外部路由自動選擇的 NIC 位址監聽並產生連結，`--public-url` 優先覆寫。這是主機 NIC 位址而非保證可從網際網路連線的 WAN 位址。
- Telegram 連結的 token 有效 24 小時且只能兌換一次，保存雜湊、以 transaction 兌換 owner session，開啟後清除 URL fragment。長期人類 token 與 agent token 不進入通知；`login --rotate` 同時撤銷未使用連結。已登入的同一瀏覽器可重開原通知。
- `/agentdesk` skill 會確保服務啟動，並教導 agent 依宿主能力建立、恢復及驗證背景 question-set monitor；活動工作期間不中斷，直到 goal 完成且無後續指示，或使用者／宿主明確停止。沒有可用的背景機制時必須回報限制，不能宣稱永久執行或自動啟動模型回合。
- Makefile 提供 build/check/test/start/install-skill 等目標。`build-all` 先建 Web，再以可設定的 bounded parallelism 建置六個 Go 目標（預設 JOBS=4）。
