# API 文件（Phase 1–2）

* Base path：`/api`（瀏覽器經 Next.js rewrites 代理至 Go backend）
* 時間：一律 UTC ISO-8601；金額：新台幣整數
* 認證：HttpOnly cookie `session`
* CSRF：所有非 GET 請求須帶 `X-CSRF-Token` header，值等於 cookie `csrf_token`
* 錯誤格式：`{"error": {"code": "BID_OUT_OF_RANGE", "message": "出價需介於 1050 ~ 1500"}}`
* 權限標記：**公開** / **登入**（不需同意條款）/ **會員**（登入且已同意最新條款）

| 狀態碼 | 意義 |
|---|---|
| 400 | 輸入驗證失敗（含未知 JSON 欄位） |
| 401 | `UNAUTHENTICATED` 未登入 |
| 403 | `TERMS_NOT_ACCEPTED`、`CSRF_FAILED`、`SELLER_CANNOT_BID`、`NOT_SELLER`、`USER_BANNED` |
| 404 | 資源不存在 |
| 409 | 狀態衝突：`AUCTION_NOT_STARTED`、`AUCTION_ENDED`、`ALREADY_HIGHEST_BIDDER`、`BID_CONFLICT`、`AUCTION_NOT_EDITABLE`、`TOO_MANY_OPEN_AUCTIONS` |
| 429 | `RATE_LIMITED` |

## 系統

### `GET /api/health` — 公開
`{"status":"ok"}`（含 DB ping）

## 認證與條款

### `GET /api/auth/line/login?next=/path` — 公開
302 至 LINE 授權頁。Mock 模式另接受 `mock_user=<英數字-_ 1~32 字>`。

### `GET /api/auth/line/callback?code&state` — 公開
驗證 state → 換取 LINE profile → 建立/更新使用者 → 建立 session → 302 至 `FRONTEND_URL + next`。
失敗時 302 至 `/login?error=<CODE>`（`INVALID_STATE`、`LINE_DENIED`、`LINE_LOGIN_FAILED`、`USER_BANNED`）。

### `POST /api/auth/logout` — 登入
204。撤銷 session 並清除 cookie。

### `GET /api/me` — 登入
```json
{
  "user": {"id":"uuid","display_name":"…","avatar_url":"https://…","role":"USER","status":"ACTIVE","created_at":"…"},
  "terms": {"current_version":"2026-10-21","accepted":false}
}
```

### `GET /api/terms` — 公開
`{"version":"2026-10-21","content":"…純文字…"}`

### `POST /api/terms/accept` — 登入
Body `{"version":"2026-10-21"}`（須等於目前版本，否則 400 `TERMS_VERSION_MISMATCH`）→ 204

## 競標

狀態由時間推導：`UPCOMING`（未開始）、`ACTIVE`（進行中）、`ENDED`（結束 < 24h）、`CLOSED`（結束 ≥ 24h，不出現在列表）。

**AuctionSummary**
```json
{"id":"uuid","title":"…","images":[{"id":"uuid","url":"/uploads/auctions/….jpg"}],
 "current_price":1050,"bid_count":1,"start_time":"…","end_time":"…","status":"ACTIVE","seller_name":"…"}
```

### `GET /api/auctions?q=&status=&page=1` — 公開
首頁列表，每頁 24 筆。`status` 可為空、`UPCOMING`、`ACTIVE`、`ENDED`。排序：進行中（最快結束優先）→ 即將開始 → 已結束。
回應 `{"items":[AuctionSummary…],"server_time":"…"}`

### `GET /api/auctions/prices?ids=uuid,uuid,…` — 公開
首頁 5 秒 polling 用，一次最多 100 個 ID。已 CLOSED 的競標不回傳。
```json
{"items":[{"id":"uuid","current_price":1100,"bid_count":2,"end_time":"…","status":"ACTIVE"}],"server_time":"…"}
```

### `GET /api/auctions/{id}` — 公開（登入時回傳個人化欄位）
```json
{"auction": {
   …AuctionSummary,
   "description":"…","start_price":0,"min_increment":50,"max_increment":500,
   "original_end_time":"…","shipping_method":"…","shipping_fee":100,
   "seller":{"id":"uuid","display_name":"…","avatar_url":"…"},
   "bids":[{"id":"uuid","amount":1050,"bidder_name":"G***o","is_mine":false,"created_at":"…"}],
   "next_bid_min":1100,"next_bid_max":1550,
   "is_seller":false,"is_highest_bidder":false,
   "contact_info":"（僅賣家可見）"
 },
 "server_time":"…"}
```

### `POST /api/auctions` — 會員
`multipart/form-data`：
* `data`：JSON
  ```json
  {"title":"豹紋守宮","description":"…","start_price":0,"min_increment":50,"max_increment":50,
   "start_time":"2026-10-21T13:00:00Z","end_time":"2026-10-23T13:00:00Z",
   "contact_info":"…","shipping_method":"黑貓宅急便","shipping_fee":150}
  ```
  `start_price`、`min_increment`、`max_increment`、`start_time`、`end_time` 可省略並套用預設（0 / 50 / 50 / 下一個整點 / 開始 + 48 小時）。
* `images`：1~4 個檔案（JPG/PNG/WebP，每張 ≤ 8MB）

限制：同時最多 5 筆 UPCOMING + ACTIVE。回應 201 `{"id":"uuid"}`

### `PUT /api/auctions/{id}` — 會員（僅賣家，僅 UPCOMING）
同建立格式，`data` 另含 `"keep_image_ids":["uuid",…]`（保留的既有圖片，依顯示順序），新上傳 `images` 接在其後；總數 1~4。回應 200 `{"id":"uuid"}`

### `POST /api/auctions/{id}/bids` — 會員
Body `{"amount":1100}`。合法範圍 `current_price + min_increment ~ current_price + max_increment`；賣家不可出價；目前最高出價者不可再出價；結束前 ≤ 1 分鐘出價，結束時間 +2 分鐘。每位使用者 1 次/秒限制。
```json
{"bid_id":"uuid","current_price":1100,"bid_count":2,"end_time":"…","extended":false,"next_bid_min":1150,"next_bid_max":1600}
```

### `GET /api/me/auctions?role=selling|bidding&status=&page=1` — 會員
我賣的 / 我出價過的競標。`status=ENDED` 包含已 CLOSED。回應同列表格式。

## 圖片

### `GET /uploads/auctions/{uuid}.jpg` — 公開
本地儲存模式下由 backend 提供（僅接受伺服器產生的 key 格式）。
