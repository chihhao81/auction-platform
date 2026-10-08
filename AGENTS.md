# Auction Toy — 開發紀錄

## 目前使用者決策（2026-10-01）

- 先完成可預覽的前端 UI。首頁的登入按鈕使用展示帳號直接進入 `/marketplace` 主畫面；這不是實際登入，不連 LINE、不呼叫 Backend。
- 市集商品、使用者、價格都是 mock data。搜尋、分類、競標狀態篩選、收藏在前端互動；真實出價、刊登、個人競標紀錄尚未實作。
- 真實 LINE Login、session、條款接受與 Neon 的端到端測試移至 Phase 1B，等前端和 Backend 都部署完成後，在部署環境驗收。不要為此在本機進行 LINE 登入或建立 HTTPS tunnel。
- 使用者條款 v1 已核可；營運者聯絡欄位依使用者指示保留待補。

## 目前階段狀態（2026-10-05）

- Phase 2 核心 Backend 與 UI 已實作：auction/bid API、狀態與價格規則、交易 row lock、圖片上傳和真實列表／詳情／建立／編輯頁面已加入工作目錄，尚待推送和部署驗收。
- Neon `production` 已套用 `db/migrations/0002_auctions_and_bids.sql`，並確認四張 Phase 2 資料表存在。真實登入流程仍依使用者決策排到部署後 Phase 1B，不在本機測。
- Phase 2 可重複出價競態整合測試已寫成 opt-in `TestConcurrentBidsAreSerializedByPostgres`；僅設定專用、可丟棄的 `AUCTION_TEST_DATABASE_URL` 才會建立並刪除隔離測試 schema。未設定時跳過，絕不可指向正式資料庫。
- Backend `go test ./...`、`go vet ./...` 通過；Frontend Vitest 11 tests 和 `npx tsc --noEmit` 通過。Vitest 在本 sandbox 使用 `--configLoader native --pool threads`；Next production build 的 TypeScript worker 仍被 Windows sandbox `spawn EPERM` 阻擋。

## 階段計畫

### Phase 1A：前端展示 UI（完成，展示入口保留）

- 品牌首頁與響應式競標市集主畫面。
- 展示登入直入主畫面；清楚揭露資料與功能為模擬。
- 搜尋、分類、狀態篩選、收藏等純前端互動。

### Phase 1B：部署後整合驗收（待部署）

- 推送程式碼至 GitHub，分別部署 Next.js 前端與 Go Backend；配置 Neon 及部署 Secrets。
- 依原始需求採用 Vercel（前端）+ OCI Always Free E2.1.Micro（Backend）+ Neon（PostgreSQL）；Vercel 以同源 rewrite 代理 `/v1/*` 至 OCI。
- 初次部署驗收不要求自有 DNS：OCI VM 可用 Cloudflare Quick Tunnel 提供暫時 HTTPS origin，填入 Vercel `BACKEND_ORIGIN`。程序停止／重啟後 URL 可能失效或改變；長期穩定服務再設定自有 DNS + TLS。LINE callback 使用 Vercel HTTPS origin。
- 前後端均有 HTTPS 網址後，設定 LINE callback。
- 僅在部署環境做 LINE Login、條款接受、session 與 DB 驗收。

### 後續

- Phase 3：通知、訂單及訂單狀態。
- Phase 4：綠界付款、出貨與訊息。
- Phase 5：評價、檢舉、管理員與 BAN。
- Phase 6：Audit Log、安全強化、測試及部署收尾。

## 技術決策

- Backend 使用 Go `net/http` + `database/sql` + pgx v5 driver；migration 使用明確版本化 SQL，不引入 ORM。
- 前端使用 Next.js App Router + TypeScript；前端 API client 位於 `frontend/src/lib/api.ts`。
- 正式 LINE Login 保留 OAuth 2.0 authorization code + OIDC，Backend 驗證 state、nonce、issuer、audience 與 ID token；Secret 僅由 Backend 部署環境提供。
- 正式 session 為 24 小時 HMAC 簽章 HttpOnly cookie；使用者條款接受狀態由 DB 控制。
- 時間在 Backend/DB 使用 UTC；前端顯示使用 Asia/Taipei。
- 同一 checkout 的 Cursor、Codex 共用 `frontend/node_modules/`；乾淨 checkout 使用 lockfile + `npm ci` 重建。

## 已完成的 Backend／資料庫

- LINE Login start/callback、ID token 驗證、PostgreSQL user upsert、HMAC session、條款目前版本／接受、logout、CORS 與 Origin 檢查已有程式碼。
- Neon `auction-platform` 專案的 `production` branch / `neondb` 已執行 `db/migrations/0001_initial.sql`，並確認 `users`、`user_terms` 存在。
- Phase 2 已實作核心功能並將版本化 migration `0002_auctions_and_bids.sql` 套用至 Neon `production`；列表／詳情／建立／編輯競標與競標出價 API 已加入。
- Phase 2 規則：UTC 時間即時計算 UPCOMING／ACTIVE／ENDED（24 小時）／CLOSED；CLOSED 不可搜尋；每位使用者最多 5 筆 UPCOMING + ACTIVE；只可編輯 UPCOMING；出價由 PostgreSQL row lock transaction 保護，延長競標亦在同交易處理。
- Phase 2 前端已有真實競標列表（5 秒單一列表輪詢）、詳情、出價與建立／編輯表單；登入尚沿用 Phase 1A 展示帳號，因此受保護 API 須 Phase 1B 部署登入後才能端到端操作。
- Phase 2 圖片上傳 API 已加入 JPEG／PNG／GIF 驗證、4 MB／尺寸限制、隨機檔名及使用者暫存額度；部署環境需配置持久化 `UPLOAD_DIR`。Neon migration 已套用；PostgreSQL 並發出價整合測試待設定專用測試資料庫後執行。
- Phase 2 本機驗證：Backend `go test ./...`、`go vet ./...` 通過；Frontend Vitest 11 tests 與 `npx tsc --noEmit` 通過。Next build 的 TypeScript worker 被 Windows sandbox `spawn EPERM` 阻擋。
- 上述登入與 DB 的部署端到端流程尚未驗收；展示 UI 不使用這些 API。
- LINE 與 Neon 的憑證曾輪替；不得抄錄到程式碼、文件、前端環境變數或 Git。部署時使用平台 Secrets。

## 過往驗證紀錄

- Backend `go test ./...` 12 項測試通過，`go vet ./...` 通過。
- 前端曾有 8 項 Vitest 通過、`tsc --noEmit` 通過、`npm audit` 0 vulnerabilities。
- Next production build 的 bundler 編譯成功，但 Windows sandbox 阻擋 TypeScript worker（`spawn EPERM`），完整 production build 當時未確認。
- Phase 2 Neon schema 已驗證；部署端到端流程尚未驗收。不要把本機 unit tests 的結果說成 LINE／部署端到端驗收。
