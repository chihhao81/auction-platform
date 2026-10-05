# 架構（Phase 1）

## 階段決策：展示版先行

目前前端採用展示模式：首頁按「使用展示帳號登入」直接進入 `/marketplace`，使用明確標示的 mock 商品資料。展示登入不建立平台身分，不呼叫 LINE 或 Backend。搜尋、分類、狀態切換和收藏為前端互動；出價、刊登與個人紀錄僅顯示尚未接通的提示。

真實 LINE Login、session、條款接受與 PostgreSQL 的前後端端到端驗收延至 Phase 1B：前端及 Go Backend 先部署到可透過 HTTPS 存取的環境，才設定正式 callback 並測試。不要為這個流程在本機建立 HTTPS tunnel 或執行真實 LINE 登入。LINE/API 程式碼可留待部署時整合。

## 元件

- Next.js App Router：呈現頁面與呼叫 API；不保存 LINE secret 或信任瀏覽器提供的身份、狀態與金額。
- Go REST API：處理認證、授權、商業規則與 PostgreSQL 存取。API 與 UI 分開部署。
- PostgreSQL：保存 users、條款接受紀錄與後續競標交易資料。migration 為版本化 SQL。

依原始需求，部署建議使用 Vercel 提供 Next.js、Oracle Cloud Always Free E2.1.Micro 執行 Go API、Neon 提供 PostgreSQL。Vercel 將 `/v1/*` 同源 rewrite 到 OCI API origin；LINE callback 亦使用 Vercel HTTPS origin，讓瀏覽器看到同源 Cookie。初次部署驗收可在 OCI VM 上用 Cloudflare Quick Tunnel 提供暫時 HTTPS origin，不需自有 DNS 網域；該 URL 在 tunnel 程序結束後失效，適合部署測試，不適合長期服務。穩定使用時再配置自有 DNS 網域與 Caddy TLS。Vercel 環境需有 `BACKEND_ORIGIN`（OCI API HTTPS origin）；前端正式 build 設定 `NEXT_PUBLIC_API_ORIGIN` 為 Vercel origin。OCI VM 內以 `HTTP_ADDR=:8080` 提供 API，由 tunnel 或反向代理轉送。

## 認證

展示版的「登入」只切換至展示市集頁，並非認證。以下為後續部署整合所保留的正式認證設計：

LINE Login 使用 OAuth 2.0 authorization code 與 OpenID Connect。Backend 產生 state/nonce 並以短效 HttpOnly cookie 綁定 callback，交換 authorization code 後呼叫 LINE ID token verify endpoint，檢查 channel ID、nonce 與 subject，再建立或更新平台使用者。Access token 不保存。LINE channel secret 只配置在 Backend。

Session 是 HMAC 簽章、24 小時效期的 HttpOnly cookie。受保護 API 每次都從 DB 載入使用者狀態及最新條款接受紀錄；BAN 或未接受最新版條款會拒絕繼續使用。登出清除瀏覽器 cookie，目前未設 server-side session revocation list。

正式環境 cookie 須設定 `Secure; HttpOnly; SameSite=Lax`（只有前後端跨 site 時才改為 `SameSite=None; Secure`），寫入 API 檢查 `Origin`，CORS 僅允許設定的 `FRONTEND_URL` origin。正式環境僅允許 HTTPS。

## Database

初始 migration 建立 `users`、`user_terms`，包含 LINE ID 唯一鍵、使用者狀態檢核、FK 與條款版本唯一鍵。所有時間使用 `timestamptz`。目前資料存取使用 `database/sql` 加 pgx v5 driver 和明確 SQL；Phase 1 查詢不需要 ORM。

條款 v1 已核可，Runtime 文案位於 `backend/terms/v1.txt`，由 `TERMS_TEXT_FILE` 讀取。若設定檔路徑無法讀取，Backend 啟動失敗。平台營運者聯絡資料尚待補入條文。

## API 慣例

- prefix：`/v1`
- JSON response；錯誤回應使用 `{ "error": "..." }`
- 時間以 UTC RFC 3339 傳輸
- 未登入、被 BAN、尚未接受最新版條款的由 Backend 查 DB 控制；目前 `/v1/me` 是第一個受保護 API
