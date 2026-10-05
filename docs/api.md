# API（Phase 1 骨架）

Base URL：部署後 Go Backend 的 HTTPS origin（本文件列出已實作的 API；目前展示版前端不呼叫它們，且不在本機測試真實 LINE Login）。

| Method | Path | 用途 | 狀態 |
| --- | --- | --- | --- |
| GET | `/v1/health` | API health check | 已實作 |
| GET | `/v1/auth/line` | 開始 LINE Login，設定 state/nonce 並導向 LINE | 已實作 |
| GET | `/v1/auth/line/callback` | 交換 code、驗證 LINE ID token、建立／更新使用者並設定 session | 已實作 |
| GET | `/v1/terms/current` | 取得目前條款版本與全文 | 已實作；未設定 TERMS_TEXT 回 503 |
| POST | `/v1/terms/accept` | 接受目前條款，JSON `{ "version": "v1" }` | 已實作；需 session、同源 Origin 及精確版本 |
| GET | `/v1/me` | 取得登入者資料 | 已實作；未接受最新條款回 428、未登入回 401 |
| POST | `/v1/logout` | 清除 session cookie | 已實作；需同源 Origin |

錯誤格式：`{"error":"message"}`，最新版條款狀態另含 `terms_version`。API 使用 credentialed CORS 且只允許設定的 `FRONTEND_URL` origin；寫入端點驗證 Origin。時間均為 UTC。

### Auth 設定與目前限制

目前 UI 使用展示帳號直入市集，不會打上述 auth API。這些端點的 LINE／DB 端到端驗收排至 Phase 1B，須待前後端部署後進行。

- LINE Login 採 authorization code + OpenID Connect；Backend 以 LINE token endpoint 交換 code，再以 LINE verify endpoint 驗證 ID token 的 issuer、`aud` channel、有效期與 nonce。Access token 不保存。
- Session 是 Backend 簽章的 HttpOnly cookie，效期 24 小時；BAN 狀態在 `/me` 查詢時由 DB 確認。此階段尚無 session 撤銷清單，登出會清除瀏覽器 cookie。
- 上線必須使用隨機 32 bytes 以上的 `SESSION_SECRET`、HTTPS、`SESSION_COOKIE_SECURE=true`。跨站前後端才選 `SESSION_COOKIE_SAMESITE=none`；需 HTTPS。
- `TERMS_TEXT` 或 `TERMS_TEXT_FILE` 提供目前條款全文；範例設定預設載入已核可的 `backend/terms/v1.txt`。設定檔路徑無法讀取時 Backend 啟動失敗。營運者聯絡欄位尚待補入。
