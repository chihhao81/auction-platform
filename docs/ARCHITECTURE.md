# 架構說明與 Database Schema

## 1. 系統架構

```
┌────────────┐  HTTPS   ┌──────────────────────────┐  rewrites /api, /uploads  ┌─────────────────────────┐
│  瀏覽器     │ ───────> │ Next.js 16（Vercel）       │ ────────────────────────> │ Go backend（Oracle VM）   │
│  + SW 推播  │          │ Client-side fetch、CSP    │                            │ chi + sqlc + pgx         │
└────────────┘          └──────────────────────────┘                            │ 單一 binary，systemd      │
      │  圖片（R2 公開網域）                                                       │ scheduler（每分鐘）        │
      └────────────────────────────────────────> Cloudflare R2 <─── 上傳 ───────── │                          │
                                                                                  └──┬──────┬──────┬────────┘
                                                                                     │      │      │
                                                              Neon PostgreSQL <──────┘      │      └──> LINE Login API
                                                                                綠界 ECPay <─┘──> Web Push 服務
```

* **瀏覽器只連前端網域**：`/api/*` 由 Next.js rewrites 代理到後端，session cookie 是 first-party，因此不需要 CORS，CSRF 用 double-submit token 處理。
* **後端是唯一的商業規則來源**：價格、出價、狀態、付款、權限全部在後端驗證；前端驗證只用於改善使用體驗。
* **沒有額外的 worker 或 queue**：競標狀態由時間即時推導；結標、合併訂單、逾期、自動收貨／完成，都由後端程序內單一的定時迴圈（預設每分鐘）處理，每個工作都是冪等的。推播在交易 commit 後以 goroutine 送出，失敗不影響主流程。

### 後端模組（`backend/internal/`）

| 套件 | 職責 |
|---|---|
| `config` | 環境變數載入與檢查（設定錯誤時拒絕啟動，例如正式環境開 Mock 登入） |
| `database` / `store` | 連線池、內嵌 migration、交易（`InTx`） |
| `database/db` | sqlc 產生的型別安全查詢（勿手改） |
| `httpx` | JSON、錯誤格式、CSRF、rate limit、安全標頭、access log |
| `auth` | LINE Login、server-side session、`RequireUser/Member/Admin` |
| `terms` | 使用者條款內文與版本 |
| `auction` | 競標規則（純函式）、發布、編輯、出價（row lock）、搜尋、價格 polling |
| `imageproc` / `storage` | 圖片驗證與重新編碼；本地磁碟 / R2 |
| `settlement` | 定時：即將結束通知、結標、合併訂單 |
| `order` | 訂單狀態機、訊息、收件資訊、出貨、收貨、完成、逾期與自動轉換 |
| `ecpay` / `payment` | 綠界簽章、結帳、callback、主動查詢、冪等 |
| `notify` | 站內通知、推播偏好、Web Push 傳送 |
| `review` / `report` / `admin` | 評價與公開個人頁、檢舉、管理後台 |
| `audit` | 只能新增的稽核紀錄（與業務變更同一交易寫入） |
| `scheduler` | 定時迴圈（不重疊、可設定逾時、會 recover panic） |

### 前端（`frontend/src/`）

* `lib/`：API client（自動帶 CSRF header）、型別、時間（一律以台北時間顯示）、polling hook（頁面隱藏時暫停）、推播、驗證工具。
* `app/`：首頁（搜尋＋每 5 秒單一 API 批次更新價格）、競標詳細與出價、發布/編輯、訂單（付款、訊息、出貨、評價）、通知、個人頁、公開個人頁、檢舉、管理後台。
* `public/sw.js`：Service Worker，處理 Web Push，只開啟同源網址。

## 2. 商業流程

```
UPCOMING ──start──> ACTIVE ──end（最後 1 分鐘出價則 +2 分鐘）──> ENDED ──24h──> CLOSED
                       │                                        │
                 管理員關閉 ───────────────> CLOSED              └─ scheduler 結標
                                                                     ├─ 無人出價：通知賣家
                                                                     ├─ 得標者已停權：不成立訂單
                                                                     └─ 得標紀錄（order_items）
                                                                          └─ 賣家 10 分鐘內無其他競標結束
                                                                             → 依買家合併成 Order

AWAITING_CONTACT ─買家第一則訊息─> AWAITING_PAYMENT ─綠界驗證成功─> PAID ─> AWAITING_SHIPMENT
      │ 24h 未聯絡：標記逾期＋系統檢舉                                     │ 買家填收件資訊 → 賣家出貨
      ▼                                                               SHIPPED ─買家確認或 3 天─> RECEIVED ─買家完成或 7 天─> COMPLETED
 管理員可取消（未付款／爭議中）→ CANCELLED                            任何進行中狀態 → DISPUTED（管理員處理）
```

## 3. Database Schema

PostgreSQL 13+（使用內建 `gen_random_uuid()`）。主鍵皆為 UUID；時間皆為 `TIMESTAMPTZ`（UTC）；金額為新台幣整數 `BIGINT`。Migration 位於 `backend/migrations/`，共 7 個版本。

| 資料表 | 主要欄位 | 一致性保護 |
|---|---|---|
| `users` | line_user_id、display_name、avatar_url、role(USER/ADMIN)、status(ACTIVE/BANNED) | `line_user_id` UNIQUE；role/status CHECK |
| `user_terms` | user_id、terms_version、accepted_at | UNIQUE(user_id, terms_version)；只新增不覆寫 |
| `sessions` | token_hash（SHA-256）、user_id、expires_at、revoked_at、ip、user_agent | token_hash UNIQUE；只存雜湊 |
| `user_bans` | user_id、reason、banned_by/at、unbanned_by/at、unban_reason | 每人最多一筆生效中（partial unique）；解除欄位需成對 |
| `auctions` | seller_id、title、description、start/min/max/current price、highest_bidder_id、bid_count、start/end/original_end_time、contact_info、shipping_method/fee、settled_at、ending_soon_notified_at、closed_at/by/reason | CHECK：max≥min、current≥start、end>start、出價者與次數一致、賣家不可為最高出價者 |
| `auction_images` | auction_id、storage_key、position(0–3)、width、height | storage_key UNIQUE；UNIQUE(auction_id, position) |
| `bids` | auction_id、bidder_id、amount | **UNIQUE(auction_id, amount)**（防同價重複出價） |
| `order_items`（得標紀錄） | auction_id、buyer_id、seller_id、amount、shipping_fee、order_id(NULL=等待合併)、won_at | auction_id UNIQUE（一個競標只會得標一次） |
| `orders` | buyer_id、seller_id、items_total、shipping_fee、total_amount、status、contact_deadline、contacted/paid/shipped/received/completed_at、overdue_at | CHECK：total = items + shipping、買賣不同人、status 列舉 |
| `payments` | order_id、merchant_trade_no、amount、status(PENDING/PAID/DUPLICATE/AMOUNT_MISMATCH)、provider_trade_no、result(JSONB 白名單欄位) | merchant_trade_no UNIQUE；**每筆訂單最多一筆 PAID**（partial unique） |
| `shipments` | order_id(PK)、收件人姓名/電話/地址、carrier、tracking_number、shipped_at、purged_at | 出貨後地址鎖定；完成 30 天後清除收件資料 |
| `messages` | order_id、sender_id、receiver_id、content(1–1000)、read_at、seq | 寄件人≠收件人；`seq` 確保順序 |
| `notifications` | user_id、type、title、content、related_auction_id、related_order_id、read_at、seq | 未讀 partial index |
| `notification_preferences` | (user_id, type) PK、push_enabled | 只存關閉的類型 |
| `push_subscriptions` | user_id、endpoint、p256dh、auth | endpoint UNIQUE；失效（404/410）自動刪除 |
| `reviews` | order_id、reviewer_id、reviewee_id、rating(POSITIVE/NEGATIVE)、comment(≤500) | UNIQUE(order_id, reviewer_id)；評價者≠被評價者 |
| `reports` | reporter_id(NULL=系統)、target_type、target_user_id、target_auction_id、order_id、reason、description、status、resolution_note、handled_by/at | 對象一致性 CHECK；同一人對同一對象只能有一筆處理中；系統檢舉同訂單同原因只建一次 |
| `audit_logs` | actor_user_id(NULL=系統)、action、target_type、target_id、metadata(JSONB)、ip、user_agent、seq | **trigger 禁止 UPDATE/DELETE** |

外鍵多數為 `ON DELETE RESTRICT`：使用者與交易資料不會被連帶刪除，BAN 也不會刪除任何歷史資料。

## 4. 資安措施總覽

| 威脅 | 對策 | 驗證方式 |
|---|---|---|
| SQL Injection | 全部使用 sqlc 參數化查詢；ILIKE 會跳脫 `%` `_` `\` | 程式掃描（沒有字串拼接 SQL）、搜尋整合測試 |
| XSS | React 一律跳脫、沒有 `dangerouslySetInnerHTML`；文字原樣儲存；CSP；頭像只接受 https | 程式掃描、訊息與描述的跳脫測試 |
| CSRF | Double-submit token（非 GET 都檢查）、SameSite=Lax、OAuth state cookie | httpx／router 測試 |
| IDOR | 訂單、訊息、評價、付款都只限買賣雙方，其他人一律 404；通知限本人 | 各模組整合測試 |
| 驗證／授權繞過 | Router 測試列舉**所有**路由，非公開路由一律要求登入；管理員路由要求 ADMIN | `server_test.go` |
| Mass assignment | JSON 一律拒絕未知欄位；DTO 只宣告允許的欄位 | httpx 測試 |
| Rate limit | 每 IP 10 rps（burst 40）；出價 1/s；訊息 1/s；檢舉約 6/分；付款 5/分/單 | 單元測試 |
| 檔案上傳 | 內容嗅探 MIME、副檔名比對、尺寸與像素上限、重新編碼為 JPEG（去除 EXIF/GPS）、伺服器產生 key | imageproc／storage 測試 |
| Path traversal | storage key 白名單格式（本地與 R2 皆檢查） | storage 測試 |
| 敏感資訊外洩 | 金鑰只放後端 env；session 只存雜湊；access log 不記 query string；錯誤訊息不含內部細節；收件地址不寫入 audit | log 測試、設定檢查 |
| Race condition | 出價 row lock＋UNIQUE；建立競標鎖 user row；條件式狀態轉換；scheduler 冪等 | 真實 DB 併發測試 |
| 付款重放／重複 callback | CheckMacValue 驗證、鎖 payment row、每單最多一筆 PAID、金額必須相符 | 付款整合測試（含 5 個併發 callback） |
| SSRF | 推播 endpoint 只接受已知 push service 的 https 網址 | notify 測試 |
| Open redirect | `next` 參數只接受站內相對路徑；付款表單只送往綠界網域 | 單元測試 |
| 依賴漏洞 | govulncheck：0 個會影響程式的漏洞（需 Go ≥ 1.27.3，build script 會強制檢查） | `govulncheck ./...` |
| 共用裝置 | 登入前先結束舊 session；登出時刪除推播訂閱 | auth 測試 |
