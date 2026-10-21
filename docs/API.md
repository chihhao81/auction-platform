# API 文件（Phase 1–4）

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

## 訂單（Phase 3）

訂單由定時 job 在結標後建立（同賣家、同買家的得標商品在賣家沒有 10 分鐘內結束的競標時合併為一筆）。只有買賣雙方可讀取，其他人回 404。

### `GET /api/orders?role=buyer|seller&page=1` — 會員
```json
{"items":[{"id":"uuid","status":"AWAITING_CONTACT","total_amount":1350,"item_count":2,
           "counterparty_name":"…","overdue":false,"created_at":"…"}]}
```

### `GET /api/orders/{id}` — 會員（僅買賣雙方）
```json
{"order":{
  "id":"uuid","status":"AWAITING_CONTACT","role":"buyer",
  "buyer":{"id":"…","display_name":"…","avatar_url":"…"},"seller":{…},
  "items":[{"auction_id":"…","title":"…","amount":500,"shipping_fee":100,"shipping_method":"…","image_url":"…"}],
  "items_total":1200,"shipping_fee":150,"total_amount":1350,
  "contact_deadline":"…","contacted_at":null,"overdue":false,
  "seller_contact":["line: …"],
  "created_at":"…"}}
```
`seller_contact` 只對買家回傳內容（賣家看到空陣列）。合併訂單運費取各商品運費最高值。

訂單狀態：`AWAITING_CONTACT` → `AWAITING_PAYMENT` → `PAID` → `AWAITING_SHIPMENT` → `SHIPPED` → `RECEIVED` → `COMPLETED`；另有 `CANCELLED`、`DISPUTED`。狀態只能由後端依規則轉換。

## 通知（Phase 3）

### `GET /api/notifications?page=1` — 會員
每頁 30 筆，新到舊。
```json
{"items":[{"id":"uuid","type":"OUTBID","title":"…","content":"…","related_auction_id":"uuid","related_order_id":null,
           "link":"/auctions/uuid","read_at":null,"created_at":"…"}],"has_more":false}
```
類型：`OUTBID`、`AUCTION_ENDING_SOON`、`AUCTION_WON`、`AUCTION_LOST`、`AUCTION_SOLD`、`AUCTION_UNSOLD`、`ORDER_WAITING`、`ORDER_CREATED`（Phase 4 起另有 `ORDER_STATUS_CHANGED`、`MESSAGE_RECEIVED`）。

### `GET /api/notifications/unread-count` — 會員
`{"count":3}`

### `POST /api/notifications/{id}/read` — 會員
204（只會影響自己的通知）

### `POST /api/notifications/read-all` — 會員
204

### `GET /api/notifications/preferences` — 會員
`{"items":[{"type":"OUTBID","label":"出價被超越","push_enabled":true}, …]}`

### `PUT /api/notifications/preferences` — 會員
Body `{"OUTBID": false, "AUCTION_WON": true}`；未知類型回 400。回應同 GET。只影響推播，站內通知一律保留。

## Web Push（Phase 3）

### `GET /api/push/public-key` — 公開
`{"public_key":"B…","enabled":true}`（VAPID 公鑰；未設定時 `enabled:false`）

### `POST /api/push/subscriptions` — 會員
Body 為瀏覽器 `PushSubscription.toJSON()`：`{"endpoint":"https://fcm.googleapis.com/…","keys":{"p256dh":"…","auth":"…"}}`。
endpoint 必須是 https 且屬於已知 push service（FCM / Mozilla / Apple / Windows）。同一 endpoint 重新訂閱時改歸目前使用者。204。

### `DELETE /api/push/subscriptions` — 會員
Body `{"endpoint":"…"}`，只刪除自己的訂閱。204。

## 訂單履約（Phase 4）

以下皆為會員 API，只有訂單的買家或賣家可呼叫，其他人一律 404。

### `GET /api/orders/{id}` 新增欄位
`paid_at`、`shipped_at`、`received_at`、`completed_at`、`auto_advance_at`（SHIPPED/RECEIVED 時系統自動推進的時間）、
`shipment`：`{"recipient_name","recipient_phone","recipient_address","carrier","tracking_number","shipped_at","purged"}`（無則 null）。

### `GET /api/orders/{id}/messages`
`{"items":[{"id","mine":true,"content":"…","created_at","read_at"}]}`，最舊在前（最多最近 200 則）；同時把對方傳給我的訊息標為已讀。

### `POST /api/orders/{id}/messages`
Body `{"content":"純文字 1~1000 字"}` → 201 `{"message":{…}}`。買家第一則訊息會讓訂單由 `AWAITING_CONTACT` 轉為 `AWAITING_PAYMENT`。訂單取消後回 409。

### `PUT /api/orders/{id}/shipping-info` — 僅買家，`AWAITING_SHIPMENT`
Body `{"recipient_name":"…","recipient_phone":"0912-345-678","recipient_address":"地址或超商門市"}` → 204。出貨後不可修改（409 `ALREADY_SHIPPED`）。

### `POST /api/orders/{id}/ship` — 僅賣家，`AWAITING_SHIPMENT`
Body `{"carrier":"黑貓宅急便","tracking_number":"…"}` → 204；買家未填收件資訊時 409 `SHIPPING_INFO_MISSING`。

### `POST /api/orders/{id}/receive` — 僅買家，`SHIPPED` → `RECEIVED`
### `POST /api/orders/{id}/complete` — 僅買家，`RECEIVED` → `COMPLETED`

自動轉換：出貨 72 小時後自動 `RECEIVED`、收貨 168 小時後自動 `COMPLETED`；訂單成立 24 小時內買家未傳訊息則標記逾期並建立系統檢舉。

## 付款（Phase 4）

### `POST /api/orders/{id}/payments` — 僅買家，`AWAITING_PAYMENT`
建立付款嘗試，回傳要自動送出的表單（欄位已由後端簽章）：
```json
{"action":"https://payment-stage.ecpay.com.tw/Cashier/AioCheckOut/V5","fields":{"MerchantID":"…","MerchantTradeNo":"CB…","TotalAmount":"1675",…,"CheckMacValue":"…"}}
```
每筆訂單每分鐘最多 5 次（429 `PAYMENT_RATE_LIMITED`）。

### `POST /api/orders/{id}/payments/sync` — 買賣雙方
後端向綠界查詢此訂單尚未完成的付款，若已付款則套用（處理 callback 延遲或遺失）。204。

### `POST /api/payments/ecpay/notify` — 綠界 server-to-server（不需登入、免 CSRF）
`application/x-www-form-urlencoded`，必須帶正確 `CheckMacValue`。回應純文字：成功或已處理 `1|OK`；簽章錯誤 400 `0|CheckMacValue Error`。付款金額必須與訂單金額相同，重複通知不會重複處理。
