# Auction Toy — 開發紀錄

## 目前使用者決策（2026-10-01）

- 先完成可預覽的前端 UI。首頁的登入按鈕使用展示帳號直接進入 `/marketplace` 主畫面；這不是實際登入，不連 LINE、不呼叫 Backend。
- 市集商品、使用者、價格都是 mock data。搜尋、分類、競標狀態篩選、收藏在前端互動；真實出價、刊登、個人競標紀錄尚未實作。
- 真實 LINE Login、session、條款接受與 Neon 的端到端測試移至 Phase 1B，等前端和 Backend 都部署完成後，在部署環境驗收。不要為此在本機進行 LINE 登入或建立 HTTPS tunnel。
- 使用者條款 v1 已核可；營運者聯絡欄位依使用者指示保留待補。

## 階段計畫

### Phase 1A：前端展示 UI（目前）

- 品牌首頁與響應式競標市集主畫面。
- 展示登入直入主畫面；清楚揭露資料與功能為模擬。
- 搜尋、分類、狀態篩選、收藏等純前端互動。

### Phase 1B：部署後整合驗收

- 推送程式碼至 GitHub，分別部署 Next.js 前端與 Go Backend；配置 Neon 及部署 Secrets。
- 依原始需求採用 Vercel（前端）+ OCI Always Free E2.1.Micro（Backend）+ Neon（PostgreSQL）；Vercel 以同源 rewrite 代理 `/v1/*` 至 OCI。
- 初次部署驗收不要求自有 DNS：OCI VM 可用 Cloudflare Quick Tunnel 提供暫時 HTTPS origin，填入 Vercel `BACKEND_ORIGIN`。程序停止／重啟後 URL 可能失效或改變；長期穩定服務再設定自有 DNS + TLS。LINE callback 使用 Vercel HTTPS origin。
- 前後端均有 HTTPS 網址後，設定 LINE callback。
- 僅在部署環境做 LINE Login、條款接受、session 與 DB 驗收。

### 後續

- Phase 2：競標商品與出價、狀態及並發保護。
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
- 上述登入與 DB 的部署端到端流程尚未驗收；展示 UI 不使用這些 API。
- LINE 與 Neon 的憑證曾輪替；不得抄錄到程式碼、文件、前端環境變數或 Git。部署時使用平台 Secrets。

## 過往驗證紀錄

- Backend `go test ./...` 12 項測試通過，`go vet ./...` 通過。
- 前端曾有 8 項 Vitest 通過、`tsc --noEmit` 通過、`npm audit` 0 vulnerabilities。
- Next production build 的 bundler 編譯成功，但 Windows sandbox 阻擋 TypeScript worker（`spawn EPERM`），完整 production build 當時未確認。
- 本次 UI／階段計畫修改尚未重新執行驗證。不得把過往驗證描述成涵蓋本次新增 UI。
