# Auction Toy

供個人使用的活體商品競標平台 Toy Version。當前優先完成可操作的前端展示流程：訪客點擊「使用展示帳號登入」後直接進入市集主畫面，不呼叫 LINE Login 或 Backend。展示商品與操作皆為模擬資料。

## 目前範圍

- `frontend/` — Next.js App Router 前端，包含品牌首頁、`/marketplace` 展示市集，以及 Phase 2 真實競標列表、詳情／出價與刊登表單頁面。
- `backend/` — Go REST API，已實作 LINE Login、session、條款接受、PostgreSQL user schema，以及 Phase 2 競標／出價 API。
- `db/migrations/` — PostgreSQL schema migrations。Neon `production` 已建立 Phase 1 與 Phase 2 所需資料表。
- `docs/` — API、架構與條款說明。

## 新階段計畫

### Phase 1A：前端展示版（完成，展示入口保留）

- 完成品牌首頁與競標市集主畫面。
- 按登入直接以展示帳號進入主畫面，不在本機執行真實 LINE Login 或前後端端到端驗證。
- 以清楚標示的模擬資料呈現商品、價格和互動。

### Phase 1B：部署後整合驗收（待部署）

- 將程式碼推至 GitHub，部署前端及 Go Backend，配置 Neon 與部署環境 Secrets。
- 在部署後的 HTTPS 網域設定 LINE Login callback。
- 僅在前後端均部署完成後，驗收 LINE Login、條款接受與 PostgreSQL session 流程。

### Phase 2（核心程式已完成，待部署驗收）

- 後端競標 CRUD、狀態計算、增量驗證、並發出價交易保護已實作。
- 前端提供真實列表、詳情與建立表單；需部署後正式 LINE 登入才可操作受保護的寫入 API。
- 安全檔案上傳已支援 JPEG／PNG／GIF；Neon migration 已套用並確認四張新資料表。圖片上限 4 MB，以預留代理路徑的 request 大小空間。
- Backend、前端單元測試及 TypeScript 檢查已通過；PostgreSQL 並發出價整合測試仍待專用測試資料庫。登入後寫入流程要等前後端部署完成再驗收。

### Phase 3–6

Phase 3 起依原需求開發通知與訂單、付款／出貨／訊息、評價／檢舉／管理／BAN，以及 Audit Log、安全強化與部署收尾。真實出價和交易規則始終由 Backend 驗證。

## 本機前端開發

```powershell
Set-Location frontend
npm ci
npm run dev
```

開啟 `http://localhost:3000`，按「使用展示帳號登入」進入 `/marketplace` 展示市集；這條流程不呼叫 LINE 或 Backend。Phase 2 的 `/auctions` 頁面會呼叫真實 API，受保護的出價／刊登需有效 LINE session 和 Neon migration；真實登入仍依使用者決定只在部署後測試。

## 部署整合注意

依原始需求，部署組合採用：**私有 GitHub repo + Vercel（Next.js）+ Oracle Cloud Always Free E2.1.Micro（Go API）+ 現有 Neon（PostgreSQL）**。GitHub Pages 是靜態網站服務，無法執行 Go API；Vercel 會以同源 rewrite 代理 API，避免瀏覽器跨站傳送登入 Cookie。

Vercel 專案根目錄設為 `frontend/`，並在 Vercel 環境變數設定：

- `BACKEND_ORIGIN`：初次部署測試填 OCI 上 Quick Tunnel 輸出的 HTTPS origin；長期部署改為穩定 API 網域。
- `NEXT_PUBLIC_API_ORIGIN`：Vercel 前端的正式 origin，例如 `https://auction-toy.vercel.app`。

OCI Backend 還需設定 `UPLOAD_DIR=/var/lib/auction-toy/uploads`，並確保目錄由 API 執行帳號持有、具有限制存取權且位於持久化磁碟。此 Toy Version 將圖片存在單一 API 主機，不做多機同步。

`frontend/next.config.ts` 會把 `/v1/*` 透過 rewrite 代理到 `BACKEND_ORIGIN`，因此瀏覽器向前端同源 `/v1/*` 發請求，LINE callback 也能使用前端 HTTPS 網域。**初次部署驗收可以先不買網域**：在 OCI VM 上跑 Cloudflare Quick Tunnel，把 `BACKEND_ORIGIN` 設成它產生的 HTTPS 網址。Quick Tunnel 無需網域或 Cloudflare 帳號，但 URL 是暫時的；程序停止後連結失效，重啟後若 URL 改變就要更新 Vercel 環境變數並重新部署。它適合這次測試，不是長期穩定的正式入口。之後若要持續使用，再設定自有網域和穩定 TLS。

OCI VM 上部署 Go API：

```sh
cd backend
go build -tags netgo -ldflags '-s -w' -o auction-api ./cmd/api
```

長期穩定部署時，以 systemd 保持 API 執行，並由 Caddy 將 HTTPS API 網域代理到本機 `127.0.0.1:8080`。OCI 網路安全清單與 VM 防火牆只需開啟必要的 SSH、HTTP、HTTPS 連線。服務環境變數需設定 `DATABASE_URL`、`LINE_CHANNEL_ID`、`LINE_CHANNEL_SECRET`、`LINE_CALLBACK_URL`、`FRONTEND_URL`、`SESSION_SECRET`、`SESSION_COOKIE_SECURE=true` 和 `SESSION_COOKIE_SAMESITE=lax`。其中 callback 設為 `https://<Vercel 網域>/v1/auth/line/callback`，`FRONTEND_URL` 設為 Vercel origin。Neon URI 與 LINE Secret 只放 OCI Backend 的環境設定／權限受限的 Secret 檔，不提交至 GitHub 或放入前端環境變數。

目前首頁登入仍是展示登入。前後端部署後，Phase 1B 再把按鈕切換成 LINE Login，設定 LINE Developers callback，驗收 LINE Login、條款接受與 PostgreSQL session。GitHub Pages 僅能提供靜態檔案；本專案使用 Vercel rewrites 將 `/v1/*` 同源代理至 OCI Go API。

Neon migrations `0001_initial.sql` 與 `0002_auctions_and_bids.sql` 已在 `auction-platform` 專案執行並驗證。條款 v1 已核可；Toy Version 的營運者聯絡欄位依使用者指示先保留待補。

## 前端依賴與其他 Agent

前端依賴安裝於 `frontend/node_modules/`；Cursor、Codex 等工具若開啟同一 checkout，就共用同一份安裝。`package-lock.json` 鎖定套件版本，應納入版本控制；不要提交 `node_modules/` 或 `.env.local`。另一個乾淨 checkout 可執行 `npm ci` 重建相同依賴，npm cache 只快取下載內容。
