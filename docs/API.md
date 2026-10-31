# Local API v1

所有 JSON 回應為 `{ "schema_version": 1, "data": ... }`；錯誤為 `{ "schema_version": 1, "code": "...", "error": "..." }`。Mutation 要求 `Content-Type: application/json`，body 上限 1 MiB，拒絕未知欄位。跨專案憑證不通用。

## 人類

`POST /api/v1/session` 接受 `{ "token": "..." }`，交換為 7 天的 HttpOnly cookie。`GET /api/v1/session` 回傳 project ID、name 與由服務決定的 `human:owner`。`DELETE /api/v1/session` 撤銷目前 session。登入 token 30 天到期，可用本機 `login --rotate` 撤銷既有 token 與所有 session。

以下端點要求人類 session；所有人類 mutation（含登入）要求 Origin 等於目前 origin 或明確設定的 public URL。不接受 agent Bearer 作為人類驗證。

| 方法／路徑 | 用途／body |
| --- | --- |
| GET `/api/v1/issues?pending=true` | 議題清單；省略 pending 顯示全部 |
| GET `/api/v1/issues/{id}` | 議題全文、revision、回答與狀態 |
| GET `/api/v1/issues/{id}/history` | 全部歷史事件及各 consumer 的 receipts |
| POST `/api/v1/issues/{id}/reply` | `{revision, option_id?, text?, ack?}` |
| POST `/api/v1/issues/{id}/defer` | `{revision, deferred}`，不回答／Ack |

Decision 至少選一個有效 option ID 或填寫非空 text。Action／Notice 要求 `ack: true`，可附 text。Actor 由已驗證的 session 決定，傳入 actor 會被拒絕。相同 revision 與相同答案重送不新增事件；不同答案、舊 revision 或撤銷議題回 409。第一版單一人類 owner，沒有多使用者帳號。

## Agent

前綴 `/api/v1/agent`，同時要求 `Authorization: Bearer AGENT_TOKEN` 與 `X-Project-ID: PROJECT_ID`。每個專案使用不同 token。

| 方法／路徑（省略前綴） | 用途／body |
| --- | --- |
| POST `/issues` | Draft，支援 `Idempotency-Key` header |
| GET `/issues`、`/issues/{id}`、`/issues/{id}/history` | 查詢 |
| POST `/issues/{id}/update` | `{revision, draft}`；pending only，revision +1 |
| POST `/issues/{id}/withdraw` | `{revision}`；pending only |
| GET `/inbox?consumer=coordinator` | 最多 200 個未 receipt 事件 |
| GET `/wait?consumer=coordinator&after=0&timeout=50s` | Long poll，伺服器單次上限 55s |
| GET `/wait?issue=ID&timeout=50s` | 等到 issue 非 pending |
| POST `/claim` | `{consumer, holder, ttl:"60s", release:false}`；ttl 1s–10m |
| POST `/receipt/{event_id}` | `{consumer, holder}`，要求有效 lease |

Draft 欄位：`kind`、`title`、`context`、`options`（`id/label/description/pros/cons`）、`recommended_option`、`recommendation_reason`、`created_by`、`owner_coordinator`、`origin_session`、`task_ref`、`blocking_scope`、`branch`、`commit`、`related_documents`、`priority`（low/normal/high）。`created_by` 與來源欄位為 agent 提供的描述，非人類身分證明。缺省 owner 為 coordinator。

Inbox 交付 answered、acked、updated、withdrawn；建立與稍後處理只列在完整歷史。每事件含穩定 `event_id`、遞增 `seq`、`issue_id`、`revision`、`kind`、`actor`、`at` 與該次 issue snapshot。每次使用最新 get 驗證工作仍適用。回答／Ack 與事件在同一 SQLite transaction 保存。

Wait 若已有事件立即回傳，逾時回傳 `data: {"timed_out": true}`，不修改狀態。Inbox／wait／watch 讀取均不自動推進進度；只有 receipt 確認指定 consumer 的單一事件。Consumer 使用同一 lease holder 更新租約；不同 holder 在到期前不能接管。Receipt 不是工作完成，也不保證外部工具 exactly-once。

狀態為 pending → answered（Decision）／acked（Action、Notice）／withdrawn。Deferred 是獨立標記。版本只在 pending issue 的實質修改時增加，已回答議題不能再改題或改答案。
