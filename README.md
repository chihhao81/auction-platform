# Critter Bid — 活體商品競標平台（Toy Version）

供個人使用的競標平台（守宮、鼠婦等活體商品）。前端 Next.js 16 部署於 Vercel，後端 Go 單一 binary 部署於 Oracle Cloud Free Tier，資料庫為 Neon PostgreSQL，圖片存放於 Cloudflare R2。

## 文件

| 文件 | 內容 |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 架構說明、商業流程、**Database Schema**、資安措施總覽 |
| [docs/API.md](docs/API.md) | **API 文件**（所有 endpoint、權限、錯誤碼） |
| [deploy/README.md](deploy/README.md) | **部署方式**（Oracle VM、Caddy、systemd、Vercel、R2、LINE、綠界） |
| [AGENTS.md](AGENTS.md) | 架構決策紀錄（ADR）、已確認決策、開發進度，供後續開發者與 AI Agent 參考 |

## 功能

* LINE Login（含開發用 Mock 模式）、使用者條款同意、server-side session
* 競標發布／編輯（開始前）、最多 4 張圖片、每人最多 5 筆開放中競標
* 出價：範圍驗證、row lock 防併發、結束前 1 分鐘出價延長 2 分鐘
* 首頁搜尋，以單一 API 每 5 秒批次更新價格
* 定時結標、同賣家 10 分鐘內結束的得標商品合併訂單
* 站內通知與 Web Push（可依類型關閉推播）
* 訂單：訊息、綠界付款（簽章驗證、冪等、主動查詢）、收件資訊、出貨、收貨、完成，以及逾期與自動轉換
* 雙向評價、公開個人頁、檢舉、管理後台（BAN／解除停權、關閉競標、取消訂單、檢舉處理、付款異常、稽核紀錄）
* 重要操作都有 Audit Log（只能新增，資料庫層禁止修改與刪除）

## 本機開發

```bash
# backend（Go 1.27.3+、Neon 或 PostgreSQL 13+）
cd backend && cp .env.example .env      # 填入 DATABASE_URL；預設 MOCK_LINE_AUTH=true
go run ./cmd/server                      # http://localhost:8080，啟動時自動 migrate

# frontend（Node 24）
cd frontend && cp .env.example .env.local && npm install
npm run dev                              # http://localhost:3000
```

Mock 登入：`http://localhost:3000/api/auth/line/login?mock_user=alice`。`.env.example` 預設 `ADMIN_LINE_USER_IDS=mock-admin`，所以用 `mock_user=admin` 登入即為管理員。

## 測試

```bash
cd backend
go test ./...                                  # 單元測試（不需資料庫，整合測試會自動 skip）
TEST_DATABASE_URL=postgres://... go test ./... # 加上真實 PostgreSQL 整合測試（每個測試使用獨立 schema，結束後刪除）
govulncheck ./...

cd frontend
npm test && npm run typecheck && npm run lint && npm run build
```

### 測試結果（2026-10-22）

| 項目 | 結果 |
|---|---|
| 後端 | 18 個 package 全數通過；164 個測試（含子測試），其中 **28 個是在 Neon PostgreSQL 18 上執行的整合測試** |
| 前端 | 8 個測試檔、42 個測試全數通過；TypeScript、ESLint、production build 皆通過 |
| 依賴漏洞 | Go 依賴：0 個會影響程式的漏洞（Go 標準函式庫需 **1.27.3 以上**，build script 會強制檢查）；npm 正式依賴：0 個漏洞 |
| 瀏覽器端對端 | 每個 Phase 都實際操作驗證過（見 AGENTS.md 各 Phase 紀錄） |

重點測試涵蓋：

* 競標狀態、出價範圍、最小與最高加價、賣家不可出價、最高出價者不可連續出價
* 結標、延長、**併發出價**（10 人同時出同價只有 1 人成功；不同價格併發不會讓低價覆蓋高價）
* 訂單建立與合併、訂單狀態轉換
* 付款 callback（偽造、金額不符、**5 個重複 callback 同時到達只處理一次**、重複付款）
* 權限：所有路由的登入檢查、IDOR
* BAN、評價規則、檢舉、管理員操作、稽核紀錄只能新增

## 已知限制

1. **真實刷卡與綠界 callback 尚未在公開網址上實測**：本機開發時綠界連不到 localhost。目前已驗證：簽章（官方範例與 stage 伺服器）、stage 付款頁正確顯示、以模擬 callback 跑完整流程、主動查詢。部署後應以綠界測試卡實際走一次。
2. **重複付款需人工退款**：系統會偵測、標記並列在管理後台，但沒有串接綠界退款 API。
3. **Web Push 尚未在實體裝置上收到推播**：訂閱、偏好設定、失效訂閱刪除都已測試。iPhone 需先「加入主畫面」才能使用推播。
4. **Client IP 依賴代理標頭**：後端位於 Vercel 與 Caddy 後方，IP 來自 `CLIENT_IP_HEADER`。直接連線後端的人可以偽造這個標頭，影響範圍限於 rate limit 與 audit 紀錄的 IP 正確性；部署後需確認稽核紀錄中的 IP 正確。
5. **Rate limit 存在記憶體中**：只適用於單一實例，重新啟動後歸零。
6. **「競標即將開始」通知未實作**：因為沒有收藏／關注功能，沒有合適的收件者。
7. **金額只支援新台幣整數**；**時間一律以台北時間顯示**。
8. **CSP 使用 `unsafe-inline`**：頁面是靜態產生的，因此依 Next.js 文件採用「不使用 nonce」的設定。改用 nonce 需要讓所有頁面改為動態渲染。
9. **單一 VM、單一實例**：沒有高可用設計，符合 Toy Version 的定位。
