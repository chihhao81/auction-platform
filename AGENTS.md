# AGENTS.md - 專案決策與開發指引紀錄

本文件記錄「競標平台 Toy Version」之核心架構決策、技術選型、開發規範與階段性進度，供所有 Agent 及開發者參考遵循。

---

## 1. 專案基本定位

* **目的**：供個人使用的活體商品競標平台 Toy Version（如守宮、鼠婦等）。
* **非目標**：不考慮正式公開商業營運、大規模流量、SEO 或高併發微服務架構。
* **部署標的**：Oracle Cloud Free Tier E2 Micro (1 OCPU, 1GB RAM AMD 或 ARM)。
* **核心原則**：
  1. 程式碼品質與正確性優先於微小的記憶體最佳化。
  2. 避免不必要的常駐 Background Worker 與高頻 Polling。
  3. 任何涉及價格、付款、Order status、Auction status 的商業規則均由 Backend 掌控驗證，前端不可信。
  4. 重要操作具備 Audit Log，前後端均需具備 Unit Test。

---

## 2. 核心架構決策紀錄 (ADR)

### ADR-01: Monorepo 專案結構
* **決策**：在單一 Git 專案根目錄下分為 `frontend/` 與 `backend/`。
* **理由**：
  * 個人專案及階段性（Phase 1~6）開發中，前後端合約（DTO、狀態機、API）變更可透過單一 Git Commit 原子性提交，避免多 Repo 同步脫鉤。
  * 獨立部署完全不受影響：前端可獨立掛載至 Vercel（免費且不消耗 Oracle VM RAM），後端編譯為單一 Go Binary 部署於 Oracle Cloud VM。

### ADR-02: 資料庫選型與連線 (Neon PostgreSQL)
* **決策**：採用雲端 Serverless PostgreSQL (Neon.tech)，專案建議名稱：`critter-bid` 或 `toy-auction`。
* **理由**：
  * 免除本地安裝與背景服務常駐之 RAM/磁碟消耗。
  * 本地開發與未來部署共用連線字串，無移轉與資料匯出問題。
  * 附帶 Web 介面便於直接檢視資料表。
  * 單元測試層仍採用 Mock DB（`pgxmock`），無需網路即可執行獨立單元測試。
* **Migration**：`golang-migrate`，SQL 檔以 `embed` 打包進 binary，`MIGRATE_ON_START=true` 時啟動自動套用。需 PostgreSQL 13+（使用內建 `gen_random_uuid()`）。
* **連線池**：MaxConns 10、MinConns 0（讓 Neon 可 scale to zero）。

### ADR-03: 後端 ORM / 查詢建構器 (sqlc + pgx)
* **決策**：採用 `sqlc` 搭配 `pgx/v5`，HTTP router 使用 `chi`。
* **理由**：
  * **零反射 (Zero Reflection)**：相比 GORM，`sqlc` 在編譯期生成純 Go 結構體與函式，執行期幾乎零額外 RAM 開銷，非常適合 1GB RAM 之 VM。
  * **精確鎖定與交易控制**：競標防併發（`SELECT ... FOR UPDATE`）與多商品合併訂單等場景需嚴謹的交易邊界，原生 SQL 比 ORM 的黑箱更精確可靠。
  * **型別安全**：SQL 語法在編譯期即完成靜態型別與語意驗證。
* 修改 `backend/queries/*.sql` 或 `backend/migrations/*.sql` 後執行 `sqlc generate`；`internal/database/db/` 為產生碼，勿手改。
* 交易統一用 `store.Store.InTx`；Audit Log 必須使用同一交易的 `q` 寫入。

### ADR-04: 圖片儲存策略 (抽象 Storage 介面)
* **決策**：後端實作 `storage.Storage` 介面，支援：
  1. 本地檔案儲存（Local Storage）：本機開發與測試預設使用（已實作）。
  2. S3 相容物件儲存（建議 Cloudflare R2）：生產環境無縫接軌，享有每月 10GB 免費且免對外流量費（Zero Egress Fee），避免擠爆 Oracle E2 Micro 磁碟（Phase 6 實作）。

### ADR-05: LINE Login 身份驗證與本地 Mock
* **決策**：標準 OAuth2 Code Flow 整合 LINE Login，並在後端內建 `MOCK_LINE_AUTH=true` 開關。
* **理由**：
  * 敏感 Secret 只存在 Backend 環境變數。LINE access token 只用一次取 profile，不保存。
  * 提供 Mock 模式讓本機開發與自動化測試無需架設外部 tunnel（如 ngrok）或真實 LINE 帳號即可順暢執行。`APP_ENV=production` 時禁止開啟 Mock（設定檢查會拒絕啟動）。
  * 條款同意紀錄儲存於 `user_terms`（記錄 `user_id`, `terms_version`, `accepted_at`），只新增不覆寫（保留原始同意時間）；未接受最新條款時中介軟體 `RequireMember` 阻擋受保護 API。
  * 條款版本常數與內文同放 `backend/internal/terms/`，修改內文必須同時更新版本。
  * 管理員由環境變數 `ADMIN_LINE_USER_IDS` 指定，登入時寫入 `users.role`。Mock 模式下 LINE user ID 為 `mock-<name>`。

### ADR-06: 時區規範 (Timezone)
* **後端與資料庫**：一律採用 **UTC** 儲存與計算時間（競標開始、結束、延長判定等）。
* **前端展示**：一律轉換為 **Asia/Taipei (UTC+8)** 顯示，不得依賴客戶端本地時鐘做時間終止判定。
* **實作**：Go 端以注入的 `now func() time.Time` 取得時間（可測試）；pgx 連線設定 `timezone=UTC`；API 回應附 `server_time`，前端以此計算時鐘偏移，倒數僅供顯示。

### ADR-07: Server-side Session（取代 JWT）
* **決策**：登入後產生 32 bytes 隨機 token，存於 HttpOnly + SameSite=Lax cookie `session`；DB `sessions` 只存 SHA-256 雜湊。
* **理由**：JWT 無法即時撤銷，BAN / 登出必須立即生效。每個請求一次查詢（session + user + 是否已同意最新條款合併為單一 SQL），成本可忽略。被 BAN 的使用者在 `LoadUser` 中被視為未登入。
* `golang-jwt` 已自 go.mod 移除。

### ADR-08: 前端代理 /api 與 CSRF
* **決策**：瀏覽器只連 Next.js 網域，`next.config.ts` 以 rewrites 將 `/api/*`、`/uploads/*` 代理至 Go backend（`BACKEND_URL`）。cookie 因此為 first-party，不需 CORS。
* **CSRF**：Double-submit cookie（`csrf_token` cookie + `X-CSRF-Token` header），所有非 GET 請求檢查；未來綠界 callback 路徑以 exempt 清單排除（改以 CheckMacValue 驗證）。
* **登入 CSRF / Open redirect**：OAuth `state` 綁 HttpOnly cookie；`next` 參數只接受站內相對路徑。
* **Client IP**：`CLIENT_IP_HEADER` 設定可信代理標頭（如 `X-Forwarded-For`）。部署在 Vercel 代理後方時**必須設定**，否則所有請求共用 Vercel IP，會觸發全站 rate limit（部署時需實測 Vercel 轉送的標頭）。
* **Mass assignment**：JSON 解碼一律 `DisallowUnknownFields`，DTO 只宣告允許的欄位。

### ADR-09: 競標狀態由時間推導、不用排程翻轉
* **決策**：`UPCOMING/ACTIVE/ENDED/CLOSED` 由 `start_time`、`end_time` 與現在時間即時計算（`auction.StatusAt`；SQL 查詢使用相同條件），資料表不存 status 欄位。
* **理由**：無需 Background Worker 即可保證狀態正確；出價時在 row lock 之後才讀取時間，結標後不可能再成功出價。
* Phase 3 的「結標處理（得標紀錄、通知、合併訂單）」由每分鐘執行的定時 function 處理，見「已確認決策」第 1 點。

### ADR-10: 出價併發控制
* 交易內 `SELECT ... FROM auctions WHERE id=$1 FOR UPDATE` → 讀取時間 → 純函式 `ValidateBid` → INSERT bid → UPDATE auction（價格、最高出價者、次數、延長後 end_time）→ Audit Log → COMMIT。
* 第二道防線：`UNIQUE (auction_id, amount)`、CHECK `current_price >= start_price`、CHECK 賣家不可為最高出價者、CHECK `bid_count` 與 `highest_bidder_id` 一致。
* 每位使用者出價另有 1 次/秒（burst 3）的 rate limit。
* 建立競標時鎖 user row（`SELECT status ... FOR UPDATE`）再計數，防止併發建立超過 5 筆，並在鎖內重新確認未被 BAN。

### ADR-11: 圖片上傳
* Multipart（`data` JSON 欄位 + 最多 4 個 `images` 檔案），每張 ≤ 8MB。
* 後端以內容嗅探 MIME（不信任 Content-Type）、副檔名須與格式相符（jpg/jpeg/png/webp）、`DecodeConfig` 限制 ≤ 30MP，之後**解碼並重新編碼為 JPEG**（套用 EXIF 方向、移除 EXIF/GPS、長邊縮至 2048、透明背景補白）。一次只處理一張以限制記憶體峰值。
* 儲存 key 由伺服器產生（`auctions/<uuid>.jpg`），永不使用使用者檔名；本地 storage 只服務符合 key 格式的路徑（防 path traversal）。DB 交易失敗時刪除已上傳檔案；編輯後刪除被移除的舊圖。
* 前端上傳前會先在瀏覽器縮圖（僅為節省流量，後端仍完整驗證）。

### ADR-12: 測試策略
* **純商業規則**（狀態、出價範圍、延長、輸入驗證）以 table-driven 單元測試覆蓋。
* **交易流程**以 `pgxmock` 驗證 SQL 順序、row lock、rollback、上傳檔清理。注意：`pgxmock v4.9.0` 與 `pgx v5.11` 不相容（缺 `Rows.TypeMap`），**pgx 鎖定 v5.10.0**，升級前需確認 pgxmock 已支援。
* **真實 PostgreSQL 整合測試**（併發出價、併發建立競標上限、搜尋 SQL、延長、audit log append-only trigger）以 `TEST_DATABASE_URL` 啟用；每個測試建立獨立 schema 並在結束後 DROP，可安全指向 Neon dev branch。未設定時自動 skip。
* 前端：Vitest + Testing Library，測試 API client（CSRF header、錯誤對應）、時區轉換、出價 UX 規則、表單驗證與預設值。

### ADR-13: Next.js 16 注意事項
* 本專案使用 Next.js 16.4（`cacheComponents` + `partialPrefetching` 開啟）。與舊版差異大，開發前請閱讀 `frontend/node_modules/next/dist/docs/`（`frontend/AGENTS.md` 亦有提醒）。
* 使用 `useSearchParams()` / `usePathname()` / `useParams()` 的 Client Component 必須包在 `<Suspense>` 內，否則 build 失敗（`RequireMember` 已內建 Suspense）。
* 頁面採 Client-side fetch（無 SEO 需求），`page.tsx` 為 Server Component 外殼。
* 路由在背景時以 `<Activity>` 隱藏保留，`usePolling` 會在頁面隱藏時暫停。

---

## 3. 開發階段進度 (Roadmap)

* [x] **Phase 1: 基礎架構、使用者與驗證**（2026-10-21）
  * 專案骨架、golang-migrate、環境設定檢查
  * LINE Login OAuth2 Code Flow + Mock 模式（`/api/auth/line/login?mock_user=alice`）
  * Server-side session、登出、管理員角色
  * 使用者條款流程與 `RequireMember` 中介軟體
  * 安全基礎：CSRF、per-IP rate limit、安全標頭、嚴格 JSON
  * Audit：LOGIN / LOGIN_REJECTED_BANNED / LOGOUT / USER_CREATE / TERMS_ACCEPT（DB trigger 禁止 UPDATE/DELETE audit_logs）
  * 前端：登入、條款、個人頁、NavBar、API client
* [x] **Phase 2: 競標與出價防併發**（2026-10-21 程式與單元測試完成；**真實 DB 整合測試尚未執行**——開發環境無 PostgreSQL，需提供 `TEST_DATABASE_URL`）
  * 發布 / 編輯（僅 UPCOMING、僅賣家）競標，含預設值、圖片 1~4 張、每人最多 5 筆 UPCOMING+ACTIVE
  * 出價：範圍驗證、賣家不可出價、最高出價者不可連續出價、row lock、結束前 ≤1 分鐘出價延長 2 分鐘
  * 首頁：關鍵字（ILIKE，跳脫 `%` `_`）+ 狀態搜尋、CLOSED 不顯示、單一 API `/api/auctions/prices?ids=` 每 5 秒批次更新
  * 詳細頁（進行中時 5 秒刷新）、出價紀錄、我的競標（我賣的 / 我出價的 × 狀態）
  * Audit：AUCTION_CREATE / AUCTION_UPDATE（含欄位 diff）/ BID_PLACE（含延長資訊）
* [ ] **Phase 3: 得標、合併訂單與通知**
  * 結標判定與得標紀錄
  * 同賣家 10 分鐘內多商品合併訂單等待邏輯
  * 站內通知清單、通知偏好設定
  * Web Push API（非阻塞式推播、Subscription 表管理、410/404 刪除失效 subscription）
* [ ] **Phase 4: 綠界付款、出貨與訂單訊息**
  * Order 狀態機（AWAITING_CONTACT, AWAITING_PAYMENT, PAID...）
  * 綠界 ECPay 付款整合、後端驗證與冪等性（Idempotency）保護
  * 物流資訊登記與買賣雙方純文字訊息系統（綁定 Order）
  * 24 小時未聯絡自動標記逾期與檢舉
* [ ] **Phase 5: 評價、檢舉與管理員**
  * 雙向評價系統（正評/負評 + 留言）
  * 競標與使用者檢舉
  * 管理員後台與黑名單。BAN 歷史需獨立 `user_bans` 表（ban/unban 時間、操作者、原因）；BAN 時呼叫 `RevokeUserSessions` 撤銷所有 session
  * 管理員關閉競標：需新增 `closed_at/closed_by` 欄位並納入 `StatusAt` 與所有搜尋 SQL
* [ ] **Phase 6: Audit Log、資安強化與部署**
  * Audit Log 完整性檢查與管理員查詢介面
  * 資安防護審查（XSS, CSRF, IDOR, MIME 深度驗證等）
  * 前後端完整單元測試驗證
  * R2 storage 實作、Oracle Cloud Free Tier 部署設定與文件

---

## 4. 已採用的假設（需求未明確，可再調整）

1. **第一口出價範圍**＝`start_price + min_increment ~ start_price + max_increment`（與後續出價同一公式；不可直接以起標價出價）。
2. 預設最小加價 = 最高加價 = 50，代表預設下每次只有一個合法金額（依需求照做，表單可修改）。
3. 競標至少 1 張圖片；競標長度 10 分鐘 ~ 14 天；開始時間最晚 30 天後；金額為新台幣整數。
4. **額外聯絡方式**只對賣家顯示（Phase 3 起對得標買家於訂單中顯示），不公開於競標頁。
5. 出價紀錄的出價者名稱遮罩（如 `G***o`），本人出價標示「我」。
6. CLOSED 競標不出現在首頁與搜尋，但知道 ID 仍可開啟詳細頁（供訂單、歷史紀錄連結）。
7. 被 BAN 的賣家既有競標暫不自動下架（待 Phase 5 管理員關閉功能）。
8. 「我的競標」同時提供「我賣的」與「我出價的」兩種清單。

## 5. 已確認決策（2026-10-21 使用者確認）

1. **定時處理**：backend 內建單一定時 function，**預設每 1 分鐘**執行一次，處理到期結標（得標紀錄、通知、合併訂單等待）、通知與逾期判定。間隔以環境變數設定（部署機器太弱時可調為 5 或 10 分鐘）。每次執行需為冪等（重複執行或漏跑一輪都不可產生重複結果）。
2. **AWAITING_CONTACT → AWAITING_PAYMENT**：買家在訂單訊息傳出第一則訊息時自動轉換。
3. **PAID → AWAITING_SHIPMENT**：綠界付款 callback 經後端驗證成功後自動轉換（PAID 為中間狀態，同一交易內完成）。
4. **RECEIVED → COMPLETED**：買家按「完成」，或進入 RECEIVED 後 7 天由定時 function 自動完成。
5. **24 小時未聯絡**：訂單狀態不變，加上「逾期」標記，寫 Audit Log 並自動建立檢舉買家（交管理員處理）。
6. **買家寄件資訊**：付款後由買家於訂單填寫，僅該訂單賣家可見。
7. **第一口出價**：維持 `start_price + min_increment ~ start_price + max_increment`。

## 6. 待確認事項

1. 綠界：需要測試環境商店代號 / HashKey / HashIV（可先用綠界公開測試帳號）。
2. 「已出貨 → 已收貨」是否也需要逾時自動轉換（目前規劃：僅買家手動確認）。

## 7. 本機開發

```bash
# backend（Go 1.27、PostgreSQL 13+ 或 Neon 連線字串）
cd backend && cp .env.example .env   # 填入 DATABASE_URL
go run ./cmd/server                   # 啟動時自動 migrate
go test ./...                         # 單元測試（無需 DB）
TEST_DATABASE_URL=postgres://... go test ./...   # 加上真實 DB 整合測試
sqlc generate                         # 修改 queries/ 或 migrations/ 後

# frontend（Node 24）
cd frontend && cp .env.example .env.local
npm run dev        # http://localhost:3000，/api 代理至 :8080
npm test && npm run typecheck && npm run lint && npm run build
```

API 文件：[docs/API.md](docs/API.md)
