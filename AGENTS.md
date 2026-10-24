# AGENTS.md - 專案決策與開發指引紀錄

本文件記錄「競標平台 Toy Version」之核心架構決策、技術選型、開發規範與階段性進度，供所有 Agent 及開發者參考遵循。

---

## 0. 接手指南（新 Agent／開發者請先讀）

**目前狀態（2026-10-24）**：Phase 1~6 全部完成，已部署上線（見 ADR-22），正在由使用者做實機驗收。目前沒有進行中的功能開發。下一步工作請看 §6「待確認事項」與 §10「後續工作清單」。

**文件地圖**

| 想知道 | 看哪裡 |
|---|---|
| 為什麼這樣設計、不可違反的規則 | 本文件 §1、§2（ADR）、§4、§5 |
| 模組職責、商業流程、Schema、資安措施 | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) |
| 所有 API、錯誤格式 | [docs/API.md](docs/API.md) |
| 測試結果、已知限制 | [README.md](README.md) |
| 從零部署 | [deploy/README.md](deploy/README.md) |
| **線上環境的實際狀態與日常維運** | 本文件 ADR-22、§8 |
| 常見修改要同步改哪些地方 | 本文件 §9 |
| Next.js 16 的差異 | `frontend/AGENTS.md`、`frontend/node_modules/next/dist/docs/` |

**Agent 必須遵守的規則**

1. **溝通語言**：使用者以繁體中文溝通，回覆與文件請用繁體中文；程式碼註解維持英文（與既有程式一致）。
2. **機密資料**：LINE secret、綠界金鑰、R2 金鑰、VAPID 私鑰、資料庫連線字串、`TEST_LOGIN_PASSWORD` 只存在 backend env（本機 `backend/.env`、VM `/etc/critter-bid/backend.env`），**絕不 commit、絕不貼進對話或文件**。不要請使用者把密碼貼到對話裡。
3. **正式網站不要用密碼登入**：Agent 不可在正式網址輸入測試登入密碼；需要登入的端對端測試請在本機（localhost、Mock 登入）進行，正式環境的登入操作由使用者自己做。
4. **VM 上的 `spring-demo`（Docker，8080 port）是使用者的其他專案，不要動它**，包括 `docker` 指令、iptables 規則、Caddy 以外的服務。
5. **不可修改使用者電腦的防毒或系統安全設定**（例如替 Windows Defender 加例外），即使使用者要求也一樣。本機 Go 被 Defender 擋時，改用 CI（見 ADR-21、§8）。
6. **破壞性操作要先確認**：force push、刪除資料表／資料、改寫 git 歷史、刪除使用者檔案前都要先問；刪檔移到資源回收筒，不要永久刪除。
7. **每完成一個階段性工作**：跑測試（或讓 CI 跑）、更新本文件（決策寫成 ADR 或補進對應 ADR，進度寫進 §3／§10）、相關文件（API.md、ARCHITECTURE.md、README 已知限制）一併更新，再 commit。

**本機開發環境的限制**

* 使用者電腦（Windows 10）的 Windows Defender 會以行為偵測封鎖 `go.exe`（`go test`／`go build` 會在暫存目錄產生並執行檔案）。**本機通常無法執行 Go**；修改後端後請 push 讓 GitHub Actions 跑 gofmt／vet／test／govulncheck／build，再用 `gh run watch` 看結果。
* 前端（Node 24）可以在本機正常執行 `npm test`、`npm run typecheck`、`npm run lint`、`npm run build`。
* Shell 可用 PowerShell 與 Git Bash。`gh` 已登入（含 workflow scope），可以 push、查看 CI。

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
* **Neon pooler 注意**：`DATABASE_URL` 可用 pooled host（`ep-xxx-pooler...`）給 app 使用；但 migration 需要 session 級 advisory lock，`database.Migrate` 會自動以 `database.DirectURL` 改連 direct host。Neon 會忽略 `search_path` 啟動參數，需以 `options=-c search_path=...` 傳遞（整合測試即如此隔離 schema）。
* 2026-10-21：Neon `public` 內舊版遺留的空資料表（另一版 schema）經使用者同意已刪除，改由本專案 migration 建立。

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

### ADR-14: 定時處理與結標（Phase 3）
* `internal/scheduler`：server 內單一迴圈，啟動時先跑一次，之後每 `SCHEDULER_INTERVAL`（預設 1m，可設 10s~1h）執行；tick 不重疊、每次以 interval 為 timeout、panic 會被 recover。
* `internal/settlement` 三個 job，**皆冪等且可併發執行**（多實例或重跑都不會重複）：
  1. **即將結束通知**：`UPDATE ... SET ending_soon_notified_at ... RETURNING` 原子認領「10 分鐘內結束、有出價」的競標，通知所有出價者一次。
  2. **結標**：鎖 auction row → 再確認已結束且未結標（被延長的不處理）→ `settled_at` → 有得標者則寫 `order_items`（得標紀錄，`order_id` NULL＝等待合併）並通知得標/未得標/賣家；無人出價通知賣家。若賣家尚有 10 分鐘內結束的競標，另通知得標者「等待其他競標結束」。
  3. **成立訂單**：對有待合併得標紀錄的賣家，鎖住這些紀錄並重新檢查「賣家是否仍有未結標且於 now+10 分鐘內結束的競標」（每次重算，延長或新進入窗口都會繼續等待）；不再有時，依買家分組，每組成立一筆 `AWAITING_CONTACT` 訂單，`contact_deadline`＝成立後 24 小時。
* **合併運費**：同一筆訂單多件商品取各商品運費的**最高值**（合併寄送一個包裹）。

### ADR-15: 通知與 Web Push（Phase 3）
* 站內通知 `notifications` 與業務變更**同一交易**寫入（`notify.Batch.Add`）；交易 commit 後才 `Dispatcher.Dispatch` 推播，推播失敗只記 log，不影響主流程。
* `notify.Pusher`：goroutine 送出、最多 4 個併發、每則 30 秒 timeout；shutdown 時等待在途推播。push service 回 404/410 即刪除該 subscription。
* **SSRF 防護**：subscription endpoint 只接受 https 且主機為已知 push service（FCM、Mozilla、Apple、Windows），寫入與發送時都檢查；p256dh/auth 驗證長度（65/16 bytes）。
* 使用者可按通知類型關閉**推播**（`notification_preferences` 只存 opt-out），站內通知一律保留。
* VAPID 私鑰只在 backend env（`go run ./cmd/vapidkeys` 產生）；前端由 `GET /api/push/public-key` 取公鑰。未設定時推播停用、站內通知照常。
* 瀏覽器通知權限只在使用者按「開啟此裝置推播」時請求；登出時會刪除此瀏覽器的 subscription（共用裝置不會收到前一位使用者的推播）。
* 前端未讀數每 30 秒 polling（頁面隱藏時暫停）。

### ADR-16: 訂單狀態機
* `order.CanTransition` 定義完整轉換表；`order.Transition` 以 `UPDATE ... WHERE status = from` 條件更新（併發下只有一個成功）並寫 Audit Log。各轉換由誰觸發由各功能自行檢查（Phase 4）。
* 已付款後不可直接取消，需經 DISPUTED 由管理員處理（涉及退款）。
* 訂單只有買賣雙方可讀，其他人一律回 404（不洩漏存在與否）；賣家的「額外聯絡方式」只顯示給該訂單買家。

### ADR-17: 綠界付款（Phase 4）
* `internal/ecpay`：AIO V5。CheckMacValue＝參數依 key 排序 → 前後加 HashKey/HashIV → .NET 風格 URL encode → 小寫 → SHA256 → 大寫；已用官方文件範例驗證，並實測綠界 stage 伺服器接受我們的簽章、我們也能驗證其回應簽章。`MerchantTradeDate` 以台灣時間送出。
* **付款只認綠界簽章過的結果**：(1) server-to-server callback `POST /api/payments/ecpay/notify`（CSRF 例外，靠 CheckMacValue 驗證）；(2) 後端主動 `QueryTradeInfo`（買家回到訂單頁時觸發 `/payments/sync`，處理 callback 延遲、遺失或使用者關閉頁面）。瀏覽器回傳的任何結果都不採信。
* **冪等**：鎖 payment row，只處理 PENDING；重複/併發 callback 皆回 `1|OK` 且不重複套用。DB 另有 partial unique index 保證每筆訂單最多一筆 PAID。
* **異常**：簽章錯 → 400 `0|CheckMacValue Error`；金額不符 → `AMOUNT_MISMATCH`（不套用）；訂單已付款/不可付款時又收到付款 → `DUPLICATE`（需人工退款，Audit 標記 needs_refund）；未知交易 → 記 log 回 `1|OK`；stage 的「模擬付款」只在 `ECPAY_ALLOW_SIMULATED=true`（非 production）時接受。
* 每次付款嘗試都是新的 `MerchantTradeNo`（`CB`+UTC 時間+6 碼亂數，20 字元）；每筆訂單每分鐘最多 5 次。
* 付款成功在同一交易內 AWAITING_PAYMENT → PAID → AWAITING_SHIPMENT，通知買賣雙方。
* 本機開發 callback 打不到 localhost，靠 sync 查詢即可；部署後 `PUBLIC_BASE_URL`（預設 FRONTEND_URL）需可被綠界連到。
* `.env.example` 內是綠界官方公開的 stage 測試商店（3002607），正式環境請換成自己的商店，金鑰只放 backend env。

### ADR-18: 出貨、訊息、逾期（Phase 4）
* **訊息**：純文字 1~1000 字（`textutil.Clean` 拒絕控制字元；原文儲存、前端 escape 顯示），只有訂單買賣雙方可讀寫（他人一律 404），訂單 CANCELLED 後不可再傳。每位使用者 1 則/秒（burst 5）。對方未讀期間只發一則通知，避免洗版。
* 買家的**第一則訊息**設定 `contacted_at` 並轉為 AWAITING_PAYMENT。
* **收件資訊**存 `shipments`，付款後由買家填寫、出貨前可修改，只有該訂單買賣雙方看得到；Audit 不記錄地址內容；訂單完成或取消 `SHIPPING_INFO_RETENTION`（預設 30 天）後由定時 job 清除。
* **出貨**需已有收件資訊；記錄物流方式與單號，轉為 SHIPPED。
* **定時 job（`order.RunJobs`，與結標同一 scheduler）**：24 小時未聯絡 → 原子認領設 `overdue_at`、建立系統檢舉（`reports`，reporter 為 NULL，reason `NO_CONTACT_24H`，同訂單只會一筆）、Audit、通知雙方；SHIPPED 超過 `ORDER_AUTO_RECEIVE_AFTER`（72h）→ RECEIVED；RECEIVED 超過 `ORDER_AUTO_COMPLETE_AFTER`（168h）→ COMPLETED。
* `reports` 表已依 Phase 5 需求建立（PENDING/REVIEWING/RESOLVED/REJECTED、處理者、處理時間）。
* **排序**：同一交易內建立的列 `created_at` 相同、UUID 又是隨機的，`messages` 與 `notifications` 以 identity 欄位 `seq` 排序（migration 000005）。之後新增「依建立順序列出」的資料表也應比照。

### ADR-19: 評價、檢舉、管理員與 BAN（Phase 5）
* **評價**：綁定 COMPLETED 訂單，買賣雙方各評價對方一次（`UNIQUE(order_id, reviewer_id)`），正評/負評＋500 字內留言，送出後不可修改。公開個人頁 `/users/{id}` 顯示正評、負評、完成交易數與被評價紀錄（不含 LINE ID 或聯絡資料）。
* **檢舉**：競標（同時記錄賣家）或使用者，原因為固定選項；不可檢舉自己/自己的競標；同一檢舉人對同一對象同時只能有一筆處理中（partial unique index）。系統自動檢舉（逾期未聯絡）reporter 為 NULL。結案（RESOLVED/REJECTED）會通知檢舉人。
* **BAN**：`users.status` + `user_bans` 歷史（停權/解除的時間、操作者、原因，一人最多一筆生效中），BAN 同時撤銷所有 session，因此立即無法登入、出價、刊登、傳訊與成立訂單；不可停權管理員或自己；歷史資料不刪除。
* **得標者在結標時已被 BAN**：不建立得標紀錄與訂單，通知賣家（Audit 記錄 winner_banned）。
* **管理員關閉競標**：只限尚未結標者；設 `closed_at/closed_by/close_reason` 並同時設 `settled_at`（永不成立訂單）。`auction.StatusOf` 先判斷 `closed_at`；所有列表、價格 polling、開放中數量計算皆排除；通知賣家與所有出價者。
* **管理員取消訂單**：只限未付款（AWAITING_CONTACT/AWAITING_PAYMENT）或 DISPUTED；已付款訂單需走爭議/退款流程。
* **付款異常清單**：DUPLICATE / AMOUNT_MISMATCH 的付款列在後台，需至綠界後台人工退款或核對。
* 所有管理員寫入操作都必須填原因，與變更同交易寫 Audit（USER_BAN / USER_UNBAN / AUCTION_CLOSE / ORDER_STATUS_CHANGE / REPORT_UPDATE）。查詢操作不寫 Audit。
* **登入時先結束舊 session**：OAuth callback 在 state 驗證通過後，會先撤銷並清除瀏覽器上既有的 session，避免「被拒絕的登入（如已停權帳號）」讓前一位使用者在共用裝置上維持登入。

### ADR-20: 資安強化、R2 與部署（Phase 6）
* **路由權限守門測試**：`internal/server/server_test.go` 用 `chi.Walk` 列舉所有路由，除了 `publicRoutes` 白名單外，未登入一律必須回 401。**新增公開 API 時必須同步更新白名單**，否則測試失敗。
* **Access log**：結構化 JSON，只記 path、不記 query string（LINE callback 帶有 OAuth code）。**請求逾時**：每個請求 60 秒（同時取消 DB 查詢）。**HSTS**：只在 production 開啟。
* **前端 CSP**（`next.config.ts`）：頁面是靜態預先產生的，依 Next.js 文件採「不使用 nonce」的設定；img-src 允許 LINE 頭像網域與 `NEXT_PUBLIC_IMAGE_ORIGIN`（R2）；form-action 只允許自己與綠界；另有 Permissions-Policy。
* **R2**：`storage.S3`（minio-go）、region `auto`、單次 PUT（`DisableContentSha256`，不使用 aws-chunked）、`Cache-Control: immutable`；圖片由 R2 公開網域直接提供，不經過 VM。`STORAGE_BACKEND=local|r2`，設定不完整時拒絕啟動。
* **依賴漏洞**：已升級 golang-jwt、x/crypto。**Go 標準函式庫需 1.27.3 以上**（1.27.0～1.27.2 有 net/http、crypto 漏洞）。`scripts/build-backend.sh` 會檢查版本，不符時拒絕 build。
  * 2026-10-22：曾在 go.mod 指定 `toolchain go1.27.3` 讓 Go 自動下載，但使用者電腦的 Windows Defender 將下載的工具鏈 `go.exe` 判定為病毒並封鎖（很可能是誤判，但未繞過防毒）。因此已移除 toolchain 指定，改由 build script 檢查版本，並建議從 go.dev 下載官方安裝程式更新。
  * 2026-10-22：使用者下載 go.dev 官方安裝檔，經 SHA256 比對無誤後，以 MSI 安裝 **go1.27.3（windows/amd64）**，Defender 未再攔截。重新驗證：govulncheck 0 個會影響程式的漏洞、全部測試通過、amd64／arm64 正式 build 成功。
* **換行符號**：`.gitattributes` 強制 `*.sh`、`*.go`、`*.sql`、`deploy/` 下的設定檔使用 LF，避免在 Windows 上 checkout 後部署到 Linux 時 script 無法執行。
* **設定檔一致性測試**：`.env.example` 與 `deploy/backend.env.production.example` 中的每個設定名稱都必須是 config 實際讀取的（防止打錯字而默默使用預設值）。
* **部署**：`deploy/`（systemd 安全強化設定、`GOMEMLIMIT=300MiB`／`MemoryMax=450M`、Caddy 自動 HTTPS、正式環境設定範本、部署手冊）；`scripts/build-backend.sh amd64|arm64` 產生靜態 binary。
* `audit_logs` 加入 `seq` 排序（migration 000007）；管理後台新增「稽核紀錄」查詢（依動作、對象、操作者篩選）。
* 定時 job 另外會清除過期或撤銷超過 7 天的 session。

### ADR-21: 測試登入與 CI（部署初期）
* **測試登入**（使用者需求：初期沒有多個 LINE 帳號可扮演管理員/賣家/多個買家）：設定 `TEST_LOGIN_PASSWORD`（≥16 字元）後，登入頁出現「測試登入」，輸入任意名稱（`[a-zA-Z0-9_-]{1,32}`）＋共用密碼即可登入帳號 `test-<name>`（顯示名稱「[測試] name」）。可在 production 使用，與 Mock 不同（Mock 在 production 仍被拒絕）。
  * 密碼以 SHA-256 摘要做 constant-time 比較；每 IP 5 次後每 2 分鐘 1 次；失敗記 log；成功寫 LOGIN audit（metadata `method: test`）；登入前結束舊 session；停權、條款等規則與 LINE 帳號完全相同。未設定時端點回 404。
  * 管理員：`ADMIN_LINE_USER_IDS=test-admin`。正式上線前刪除 `TEST_LOGIN_PASSWORD` 即關閉（不需改程式）。
  * 登入方式由後端 `GET /api/auth/methods` 決定，前端不再使用 `NEXT_PUBLIC_MOCK_LOGIN`。
  * LINE 改為可選：只要有任一登入方式（LINE 三項設定、`TEST_LOGIN_PASSWORD`、或開發用 Mock）即可啟動；LINE 三項需同時設定（Mock 模式下忽略）。
* **CI**（`.github/workflows/ci.yml`）：push 到 main / PR 時在 Ubuntu 上跑 gofmt、vet、測試（有 `TEST_DATABASE_URL` secret 才跑整合測試）、govulncheck、amd64/arm64 build（產物上傳為 artifact）與前端測試/型別/lint/build。
  * 2026-10-22：使用者電腦的 Windows Defender 以行為偵測 `Behavior:Win32/DefenseEvasion.A!ml` 封鎖官方 go1.27.3 的 `go.exe`（Go 在暫存目錄產生並執行測試/編譯檔的行為常被誤判）。未替使用者加入例外；正式 binary 改由 CI 產生。本機要再跑 Go 需使用者自行在「保護歷程記錄」允許。

### ADR-22: 實際部署狀態（2026-10-22）
* **VM**：Oracle `VM.Standard.E2.1.Micro`（Ubuntu 24.04、x86_64、954MB RAM、已有 2GB swap），SSH `ubuntu@161.33.186.104`（金鑰 `~/.ssh/oracle/ssh-key-2026-09-21.key`）。
* **同機的其他服務**：使用者既有的 `spring-demo`（Docker Compose，Spring + PostgreSQL，佔用 8080）。**不要動它**；本專案後端使用 **8081**（`PORT=8081`），iptables 只開放 22/80/443。
* **網址**：無自有網域，API 使用 `https://161-33-186-104.sslip.io`（Caddy 自動 HTTPS；VM 的 IP 若變更需同步修改 Caddyfile、Vercel 的 `BACKEND_URL`）。前端使用 Vercel（使用者決定）。
* **後端**：`/opt/critter-bid/critter-bid`（由 **GitHub Actions** 產生的 amd64 binary）、systemd `critter-bid`、設定 `/etc/critter-bid/backend.env`（640 root:critterbid）、圖片存 `/var/lib/critter-bid/uploads`（暫不使用 R2）。資料庫為 Neon 同專案的 `critterbid_prod`（與開發用的 `neondb` 分開）。
* **登入**：目前只有測試登入（`TEST_LOGIN_PASSWORD` 在 VM 上產生，查詢：`sudo grep ^TEST_LOGIN_PASSWORD= /etc/critter-bid/backend.env`）；`ADMIN_LINE_USER_IDS=test-admin`。LINE 尚未設定。
* **綠界**：使用官方 stage 測試商店（不會產生真實交易）。
* **VAPID 金鑰**在 VM 上以 openssl 產生，只存在 VM。
* **Caddy 日誌**寫入 systemd journal（`journalctl -u caddy`）；寫檔到 `/var/log/caddy` 在此 VM 上會權限錯誤，已改用 journal。
* **GitHub**：`main` 已替換為本版本；原本另一份實作（Phase 1A＋2）備份於 `archive/remote-main-1008`。
* **正式網址**：前端 **https://auction-platform-zeta-flame.vercel.app**（Vercel 專案 `auction-platform`，Root Directory `frontend`，環境變數 `BACKEND_URL=https://161-33-186-104.sslip.io`）。後端 `FRONTEND_URL`／`PUBLIC_BASE_URL` 已設為此網址。
* **網路**：Oracle VCN Default Security List 已開放 TCP 80/443；Caddy 已取得 Let's Encrypt 憑證。已驗證：經 Vercel 代理到後端正常、後端記錄的 client IP 為訪客真實 IP（`CLIENT_IP_HEADER=X-Real-IP` 正確）、CSP／HSTS 生效。
* **尚待驗收**（需使用者登入操作）：測試登入→刊登→多帳號出價→結標→訂單→綠界 stage 測試卡付款（callback 現在可抵達）→出貨→評價；手機 Web Push。

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
* [x] **Phase 2: 競標與出價防併發**（2026-10-21 完成；Neon PostgreSQL 18 整合測試 6/6 通過；瀏覽器端對端驗證：mock 登入→條款→發布→出價→首頁批次 polling→Audit Log）
  * 發布 / 編輯（僅 UPCOMING、僅賣家）競標，含預設值、圖片 1~4 張、每人最多 5 筆 UPCOMING+ACTIVE
  * 出價：範圍驗證、賣家不可出價、最高出價者不可連續出價、row lock、結束前 ≤1 分鐘出價延長 2 分鐘
  * 首頁：關鍵字（ILIKE，跳脫 `%` `_`）+ 狀態搜尋、CLOSED 不顯示、單一 API `/api/auctions/prices?ids=` 每 5 秒批次更新
  * 詳細頁（進行中時 5 秒刷新）、出價紀錄、我的競標（我賣的 / 我出價的 × 狀態）
  * Audit：AUCTION_CREATE / AUCTION_UPDATE（含欄位 diff）/ BID_PLACE（含延長資訊）
* [x] **Phase 3: 得標、合併訂單與通知**（2026-10-21；Neon 整合測試 11/11 通過）
  * 定時結標、得標紀錄（`order_items`）、10 分鐘合併等待與重算、訂單成立
  * 通知：出價被超越、即將結束、得標、未得標、賣家結標/無人出價、等待合併、訂單成立
  * 站內通知清單、未讀數、全部已讀、推播偏好設定
  * Web Push（VAPID、subscription 管理、失效自動刪除、SSRF 白名單）
  * 訂單狀態機、訂單列表 / 詳細頁（買賣雙方）
  * Audit：AUCTION_SETTLE / ORDER_CREATE（系統操作 actor 為 NULL）/ ORDER_STATUS_CHANGE
* [x] **Phase 4: 綠界付款、出貨與訂單訊息**（2026-10-22；整合測試 10 項通過；瀏覽器端對端：訊息→付款頁（綠界 stage 正確顯示金額）→模擬簽章 callback→收件資訊→出貨→收貨→完成）
  * 綠界付款（簽章、callback、主動查詢、冪等、重複付款/金額不符/偽造簽章處理）
  * 訂單訊息、首則訊息觸發聯絡、收件資訊（隱私與定期清除）、出貨、收貨、完成
  * 24 小時逾期標記＋系統檢舉、自動收貨（3 天）、自動完成（7 天）
  * Audit：PAYMENT_CREATE / PAYMENT_STATUS_CHANGE / SHIPPING_INFO_SET / SHIPMENT_CREATE / ORDER_OVERDUE / ORDER_STATUS_CHANGE
* [x] **Phase 5: 評價、檢舉與管理員**（2026-10-22；整合測試新增 7 項；瀏覽器端對端：評價→公開頁→檢舉→後台結案通知→停權（無法登入）→關閉進行中競標（出價者收到通知、無法出價、首頁隱藏）→解除停權）
  * 雙向評價、公開個人頁、評分摘要
  * 競標/使用者檢舉、我的檢舉紀錄、管理員處理與通知
  * 管理後台：檢舉、使用者（BAN/UNBAN、停權歷史）、競標（關閉）、訂單（取消未付款）、付款異常
  * 修正：登入前結束舊 session（共用裝置安全）
* [x] **Phase 6: Audit Log、資安強化與部署**（2026-10-22；後端 18 個 package／164 個測試，其中 28 個為 Neon 整合測試；前端 42 個測試；以正式模式 build＋CSP 實際操作驗證）
  * Audit Log 涵蓋確認、管理員查詢介面
  * 資安審查（見 docs/ARCHITECTURE.md §4）：路由權限守門測試、access log 不記 query、請求逾時、HSTS、前端 CSP、govulncheck 與依賴升級
  * R2 storage、部署設定（systemd、Caddy、build script、正式環境設定範本）與部署手冊
  * 交付文件：README.md（測試結果、已知限制）、docs/ARCHITECTURE.md（架構、Schema、資安）、docs/API.md、deploy/README.md

**全部 Phase 已完成。** 後續若繼續開發，請先閱讀 README.md 的「已知限制」。

---

## 4. 已採用的假設（需求未明確，可再調整）

1. **第一口出價範圍**＝`start_price + min_increment ~ start_price + max_increment`（與後續出價同一公式；不可直接以起標價出價）。
2. 預設最小加價 = 最高加價 = 50，代表預設下每次只有一個合法金額（依需求照做，表單可修改）。
3. 競標至少 1 張圖片；競標長度 10 分鐘 ~ 14 天；開始時間最晚 30 天後；金額為新台幣整數。
4. **額外聯絡方式**只對賣家顯示（Phase 3 起對得標買家於訂單中顯示），不公開於競標頁。
5. 出價紀錄的出價者名稱遮罩（如 `G***o`），本人出價標示「我」。
6. CLOSED 競標不出現在首頁與搜尋，但知道 ID 仍可開啟詳細頁（供訂單、歷史紀錄連結）。
7. 被 BAN 的賣家既有競標不會自動下架，由管理員視情況「關閉競標」。
8. 「我的競標」同時提供「我賣的」與「我出價的」兩種清單。

## 5. 已確認決策（2026-10-21 使用者確認）

1. **定時處理**：backend 內建單一定時 function，**預設每 1 分鐘**執行一次，處理到期結標（得標紀錄、通知、合併訂單等待）、通知與逾期判定。間隔以環境變數設定（部署機器太弱時可調為 5 或 10 分鐘）。每次執行需為冪等（重複執行或漏跑一輪都不可產生重複結果）。
2. **AWAITING_CONTACT → AWAITING_PAYMENT**：買家在訂單訊息傳出第一則訊息時自動轉換。
3. **PAID → AWAITING_SHIPMENT**：綠界付款 callback 經後端驗證成功後自動轉換（PAID 為中間狀態，同一交易內完成）。
4. **SHIPPED → RECEIVED**：買家確認收貨，或出貨後 **3 天**由定時 function 自動轉換（天數可用環境變數調整）。
5. **RECEIVED → COMPLETED**：買家按「完成」，或進入 RECEIVED 後 7 天由定時 function 自動完成。
6. **24 小時未聯絡**：訂單狀態不變，加上「逾期」標記，寫 Audit Log 並自動建立檢舉買家（交管理員處理）。
7. **買家寄件資訊**：付款後由買家於訂單填寫，僅該訂單賣家可見。
8. **第一口出價**：維持 `start_price + min_increment ~ start_price + max_increment`。
9. **合併訂單運費**取最高值（見 ADR-14），如需改為加總再調整 `settlement.totals`。

## 6. 待確認事項

1. 綠界：目前使用官方公開 stage 測試商店；正式上線前需換成自己的商店代號與金鑰，並確認 ReturnURL 可被綠界連到。尚未在 stage 實際刷卡走完 callback（需公開網址），部署後應實測一次。
2. 「競標即將開始」通知：目前沒有「關注 / 收藏」功能，無合適收件者，尚未實作（使用者已確認先不做）。
3. DUPLICATE 付款目前只列出待人工退款，未串接綠界退款 API。
4. **賣家撥款（2026-10-23 提出，尚未決定）**：目前只串接一組綠界商店，買家付的錢全部進入「平台經營者」的綠界帳戶，系統**沒有任何撥款給賣家的功能**（沒有賣家收款帳戶、撥款紀錄或後台撥款流程）。已向使用者說明三個方向，等使用者選擇：
   * A. 平台經營者是唯一賣家：維持現狀。
   * B. 平台代收、人工轉給賣家：要做「撥款管理」（賣家收款帳戶、已完成待撥款清單、後台標記已撥款＋Audit）。在台灣可能涉及《電子支付機構管理條例》的代收轉付規範，大量或長期使用前應先確認法規。
   * C. 綠界平台商／特約會員模式（AIO 有 `PlatformID`、`PlatformChargeFee` 參數）：錢直接撥給各賣家，但需要先跟綠界簽約、每位賣家各自申請。細節尚未查證，需要先問綠界業務。

## 7. 本機開發

```bash
# backend（Go 1.27.3+、PostgreSQL 13+ 或 Neon 連線字串）
cd backend && cp .env.example .env   # 填入 DATABASE_URL
go run ./cmd/server                   # 啟動時自動 migrate
go test ./...                         # 單元測試（無需 DB）
# 整合測試：在 backend/.env 設定 TEST_DATABASE_URL，然後
export TEST_DATABASE_URL="$(grep ^TEST_DATABASE_URL= .env | cut -d= -f2-)" && go test ./...
sqlc generate                         # 修改 queries/ 或 migrations/ 後

# frontend（Node 24）
cd frontend && cp .env.example .env.local
npm run dev        # http://localhost:3000，/api 代理至 :8080
npm test && npm run typecheck && npm run lint && npm run build
```

本機 Mock 登入：backend `.env` 設 `MOCK_LINE_AUTH=true`，前端登入頁即可用任意名稱登入（`mock-<name>`）；`ADMIN_LINE_USER_IDS=mock-admin` 可當管理員。本機 Go 被 Defender 擋時無法啟動後端，見 §0。

API 文件：[docs/API.md](docs/API.md)

---

## 8. 線上環境維運手冊

環境概況見 ADR-22。以下指令的 SSH 金鑰為 `~/.ssh/oracle/ssh-key-2026-09-21.key`，主機 `ubuntu@161.33.186.104`。

### 8.1 更新後端（實際使用的流程）

本機無法 build Go，正式 binary 一律使用 CI 產物：

```bash
git push                                   # 觸發 .github/workflows/ci.yml
gh run watch                               # 等 backend job 全部通過
gh run download <run-id> -n critter-bid-linux -D dist/ci
scp -i ~/.ssh/oracle/ssh-key-2026-09-21.key dist/ci/critter-bid-linux-amd64 ubuntu@161.33.186.104:/tmp/critter-bid
ssh -i ~/.ssh/oracle/ssh-key-2026-09-21.key ubuntu@161.33.186.104 \
  'sudo cp /opt/critter-bid/critter-bid /opt/critter-bid/critter-bid.prev && sudo install -m 755 /tmp/critter-bid /opt/critter-bid/critter-bid && sudo systemctl restart critter-bid && sleep 3 && systemctl is-active critter-bid'
curl -s https://auction-platform-zeta-flame.vercel.app/api/health   # {"status":"ok"}
```

* 新版本若含 migration，啟動時會自動套用（`MIGRATE_ON_START=true`）。**含 migration 的版本要先確認 migration 可向下相容**，因為回滾 binary 不會自動回滾資料庫。
* **回滾**：`sudo install -m 755 /opt/critter-bid/critter-bid.prev /opt/critter-bid/critter-bid && sudo systemctl restart critter-bid`。
* `deploy/README.md` §4~§5 的「本機 build＋scp」是通用說明；在使用者這台電腦上請用上面的 CI 流程。

### 8.2 更新前端

push 到 `main` 後 Vercel 會自動部署（Root Directory `frontend`）。可用 `gh api repos/:owner/:repo/deployments` 或 Vercel 後台確認。前端環境變數只有 `BACKEND_URL`（必要時加 `NEXT_PUBLIC_IMAGE_ORIGIN`），**不要放任何金鑰**。

### 8.3 常用維運指令（在 VM 上）

```bash
systemctl status critter-bid
journalctl -u critter-bid -f                  # 後端 log（JSON；access log 不含 query string）
journalctl -u critter-bid --since "1 hour ago" | grep '"level":"ERROR"'
journalctl -u caddy -f                        # Caddy log
sudo systemctl restart critter-bid            # 修改 /etc/critter-bid/backend.env 後要重啟
sudo nano /etc/critter-bid/backend.env        # 640 root:critterbid，改完確認權限沒變
free -m; systemctl show critter-bid -p MemoryCurrent
```

* **VM 上的 Caddyfile 與 repo 範本不同**：`/etc/caddy/Caddyfile` 的站點是 `161-33-186-104.sslip.io`、反向代理到 `127.0.0.1:8081`；repo 內 `deploy/Caddyfile` 是 `api.example.com`／8080 的範本。修改前先看 VM 上的實際檔案，改完 `sudo caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile && sudo systemctl reload caddy`。
* **新增設定值**：先在 config 程式與兩個範本檔加上（§9），部署新 binary 前把值加進 VM 的 `backend.env`，否則可能啟動失敗（設定檢查會列出錯誤）。
* **資料庫**：正式資料在 Neon 的 `critterbid_prod`，開發與整合測試用 `neondb`（整合測試會建立並刪除獨立 schema）。**不要在正式資料庫跑整合測試或手動修改資料**；`audit_logs` 有 trigger 禁止 UPDATE/DELETE。
* **VM IP 變更**時要同步：VM 上的 Caddyfile、Vercel 的 `BACKEND_URL`、本文件 ADR-22。

### 8.4 上線前要做的設定切換（照順序）

1. LINE Login：建立 channel，Callback URL 設為 `https://auction-platform-zeta-flame.vercel.app/api/auth/line/callback`，在 VM env 填 `LINE_CHANNEL_ID`／`LINE_CHANNEL_SECRET`／`LINE_REDIRECT_URI`；用自己的 LINE 登入一次後，把自己的 `line_user_id` 填入 `ADMIN_LINE_USER_IDS`。
2. 刪除 VM env 的 `TEST_LOGIN_PASSWORD` 並重啟（測試帳號 `test-*` 的資料會保留，可由管理員停權）。
3. 綠界改用自己的正式商店（`ECPAY_MERCHANT_ID`／`HASH_KEY`／`HASH_IV`／`ECPAY_BASE_URL`），確認 `ECPAY_ALLOW_SIMULATED` 為 false，並先決定 §6 第 4 點的賣家撥款方式。
4. 有自有網域時：前端綁 Vercel 網域，更新 `FRONTEND_URL`／`PUBLIC_BASE_URL`、LINE callback。

---

## 9. 常見修改的同步檢查清單

| 修改 | 需要同步的地方 |
|---|---|
| 新增／修改 SQL 查詢 | `backend/queries/*.sql` → `sqlc generate`（產生 `internal/database/db/`，勿手改）→ 有 pgxmock 測試的套件要更新預期 SQL |
| 新增 migration | `backend/migrations/00000N_<name>.up.sql`＋`.down.sql`（編號接續，目前到 000007）→ `sqlc generate` → docs/ARCHITECTURE.md §3 Schema。需「依建立順序列出」的表要加 identity `seq`（ADR-18） |
| 新增 API | handler → `internal/server/server.go` 掛路由並套 `RequireUser/RequireMember/RequireAdmin`；**公開路由**要加進 `server_test.go` 的 `publicRoutes` → docs/API.md → 前端 `lib/types.ts`。非 GET 自動檢查 CSRF；外部 callback 才放進 CSRF exempt 清單 |
| 新增 JSON 請求欄位 | DTO 只宣告允許的欄位（解碼為 `DisallowUnknownFields`），商業規則在後端驗證 |
| 新增重要寫入操作 | 在同一交易用 `q` 寫 Audit（`internal/audit/audit.go` 新增 `Action*` 常數）；管理員操作必須帶原因 |
| 新增通知類型 | `internal/notify/notify.go` 的 `Type` 常數＋`Types`（推播偏好清單與中文標籤）；用 `notify.Batch.Add` 在交易內寫入，commit 後才 `Dispatch` |
| 新增環境變數 | `internal/config/config.go`（含驗證）→ `backend/.env.example` → `deploy/backend.env.production.example`（`config_test.go` 會檢查範本中的每個名稱都是 config 實際讀取的）→ VM `backend.env` |
| 新增定時工作 | 寫成冪等、可併發（原子認領或 row lock），加進 `settlement.RunOnce` 或 `order.RunJobs`；不要新增常駐 worker |
| 修改條款內文 | `internal/terms/terms.md` 與 `terms.go` 的 `CurrentVersion` **同時改**（所有使用者會被要求重新同意） |
| 修改訂單狀態流程 | `order.CanTransition` 轉換表＋測試，經 `order.Transition` 更新（含 Audit）；同步 §5、ADR-16 |
| 新增前端頁面 | `page.tsx` 為 Server Component 外殼；使用 `useSearchParams/usePathname/useParams` 的 Client Component 要包 `<Suspense>`；需要登入的頁面用 `RequireMember` |
| 前端載入新的外部資源 | 更新 `next.config.ts` 的 CSP（img-src、connect-src、form-action 等） |
| 依賴升級 | pgx 鎖在 v5.10.0（ADR-12，pgxmock 相容性）；Go 標準函式庫至少 1.27.3；升級後看 CI 的 govulncheck |

---

## 10. 後續工作清單（依建議優先順序）

1. **使用者實機驗收**（進行中）：測試登入 → 刊登 → 多帳號出價 → 結標 → 訂單 → 訊息 → 綠界 stage 測試卡付款（確認 callback 抵達，`journalctl -u critter-bid | grep ecpay`）→ 填收件資訊 → 出貨 → 收貨 → 評價；手機 Web Push。發現問題時先查 VM log。
2. 決定賣家撥款方式（§6 第 4 點），必要時實作撥款管理。
3. 上線前設定切換（§8.4）。
4. 視需要：重複付款串接綠界退款 API、改用 R2 存圖片（`STORAGE_BACKEND=r2`）、GitHub secret `TEST_DATABASE_URL`（讓 CI 跑整合測試，指向 Neon 的拋棄式 branch，不要指向正式資料庫）。

**最近的修正紀錄**

* 2026-10-23 `b8a391f`：發布競標時選擇圖片沒有反應。原因是 `<input type="file">` 的 `FileList` 是 live 物件，清空 input 後 state updater 才讀取，讀到空清單。現在先 `Array.from` 複製（`AuctionForm.addFiles`），並加上測試。
