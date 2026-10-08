# API（Phase 1–2）

API 由 Go Backend 提供；Phase 2 真實競標頁呼叫競標端點，Phase 1A `/marketplace` 仍保留獨立展示 mock。部署後由 Vercel 同源 rewrite 代理 `/v1/*`；真實 LINE Login 僅在部署後驗收。

| Method | Path | 用途 | 狀態 |
| --- | --- | --- | --- |
| GET | `/v1/health` | API health check | 已實作 |
| GET | `/v1/auth/line` | 開始 LINE Login，設定 state/nonce 並導向 LINE | 已實作 |
| GET | `/v1/auth/line/callback` | 交換 code、驗證 LINE ID token、建立／更新使用者並設定 session | 已實作 |
| GET | `/v1/terms/current` | 取得目前條款版本與全文 | 已實作；未設定 TERMS_TEXT 回 503 |
| POST | `/v1/terms/accept` | 接受目前條款，JSON `{ "version": "v1" }` | 已實作；需 session、同源 Origin 及精確版本 |
| GET | `/v1/me` | 取得登入者資料 | 已實作；未接受最新條款回 428、未登入回 401 |
| POST | `/v1/logout` | 清除 session cookie | 已實作；需同源 Origin |
| GET | `/v1/auctions` | 查詢 UPCOMING／ACTIVE／ENDED 競標；支援 `q` 與 `status` | 已實作；CLOSED 不回傳且不可搜尋 |
| GET | `/v1/auctions/{id}` | 競標詳情及最近 50 筆出價 | 已實作；CLOSED 回 404 |
| POST | `/v1/auctions` | 建立競標；回 `{ "id": 123 }` | 已實作；需登入、最新條款與同源 Origin；成功回 201 |
| PATCH | `/v1/auctions/{id}` | 編輯尚未開始的自有競標 | 已實作；需登入、最新條款與同源 Origin |
| POST | `/v1/auctions/{id}/bids` | 以 `{ "amount": 1050 }` 出價 | 已實作；需登入、最新條款與同源 Origin；由交易及 row lock 保護 |
| POST | `/v1/uploads` | 上傳一張競標圖片，multipart 欄位 `image` | 已實作；需登入；JPEG／PNG／GIF、4 MB、圖片內容及尺寸驗證 |
| GET | `/v1/uploads/{id}` | 取得已附加到未 CLOSED 競標的圖片 | 已實作；使用伺服器隨機 ID，不採用使用者檔名 |
| DELETE | `/v1/uploads/{id}` | 刪除尚未附加競標的自有圖片 | 已實作；需登入及同源 Origin |

錯誤格式：`{"error":"message"}`，最新版條款狀態另含 `terms_version`。API 使用 credentialed CORS 且只允許設定的 `FRONTEND_URL` origin；寫入端點驗證 Origin。時間均為 UTC。

### Auth 設定與目前限制

`/marketplace` 展示頁使用展示帳號直入市集，不會打上述 auth API。Phase 2 `/auctions` 的出價、刊登、圖片上傳與編輯呼叫受保護 API；真實 LINE／DB 端到端驗收排至 Phase 1B，須待前後端部署後進行。

- LINE Login 採 authorization code + OpenID Connect；Backend 以 LINE token endpoint 交換 code，再以 LINE verify endpoint 驗證 ID token 的 issuer、`aud` channel、有效期與 nonce。Access token 不保存。
- Session 是 Backend 簽章的 HttpOnly cookie，效期 24 小時；BAN 狀態在 `/me` 查詢時由 DB 確認。此階段尚無 session 撤銷清單，登出會清除瀏覽器 cookie。
- 上線必須使用隨機 32 bytes 以上的 `SESSION_SECRET`、HTTPS、`SESSION_COOKIE_SECURE=true`。跨站前後端才選 `SESSION_COOKIE_SAMESITE=none`；需 HTTPS。
- `TERMS_TEXT` 或 `TERMS_TEXT_FILE` 提供目前條款全文；範例設定預設載入已核可的 `backend/terms/v1.txt`。設定檔路徑無法讀取時 Backend 啟動失敗。營運者聯絡欄位尚待補入。

### Auction／Bid

- 價格與運費使用新台幣整數（BIGINT）。建立競標時預設起標價 0、最小／最大加價 50、開始時間為下一個整點、結束時間為開始後 48 小時；結束必須晚於開始。
- 每位賣家最多 5 筆 UPCOMING + ACTIVE。建立時以 PostgreSQL transaction advisory lock 序列化同一賣家的名額檢查。
- 出價只能發生在 ACTIVE 狀態；賣家與目前最高出價者不可再出價。合法範圍為目前價 + 最小加價至目前價 + 最大加價（含邊界）。競標 row lock 將記錄出價、更新最高價／出價者與延長時間放在同一交易。
- 距結束不超過 1 分鐘的合法出價會將結束時間延長 2 分鐘。狀態依 UTC 時間計算：UPCOMING、ACTIVE、ENDED（結束後 24 小時內）、CLOSED。CLOSED 不出現在列表且詳情回 404。
- 圖片上傳限制 JPEG／PNG／GIF、單檔 4 MB、最大 10000×10000 且 2000 萬像素、每筆競標最多 4 張；同一使用者最多暫存 10 張／50 MB 未使用圖片。後端檢查副檔名、偵測 MIME、解碼圖片標頭及像素尺寸，忽略原始檔名，以隨機 ID 儲存在 `UPLOAD_DIR`。OCI 部署需將該目錄設為持久化磁碟路徑；圖片只在附加至競標且競標未 CLOSED 時公開讀取。
- 套用 schema 時，在 Neon SQL Editor 執行 `db/migrations/0002_auctions_and_bids.sql`。這個 migration 不會自動執行。
- 商品圖片的 `contact_method` 只會在本人以有效 session 查看自己的競標詳情時回傳；公開列表與其他使用者的詳情不會揭露額外聯絡方式。
