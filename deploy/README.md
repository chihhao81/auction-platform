# 部署手冊（Oracle Cloud Free Tier + Vercel + Neon + Cloudflare R2）

```
瀏覽器 ──HTTPS──> Vercel（Next.js 前端，app.example.com）
                    │  /api/* rewrites
                    ▼
              Caddy（HTTPS，api.example.com） ──> Go backend :8080（Oracle VM，systemd）
                                                   ├─> Neon PostgreSQL
                                                   ├─> Cloudflare R2（圖片上傳）
                                                   ├─> LINE Login API
                                                   └─> 綠界 ECPay
瀏覽器 ──HTTPS──> R2 公開網域（img.example.com，直接讀圖片，不經過 VM）
綠界 ──付款通知──> app.example.com/api/payments/ecpay/notify ──(Vercel 代理)──> backend
```

以下 `app.example.com`、`api.example.com`、`img.example.com` 請換成你的網域。若沒有網域，前端可用 Vercel 給的 `xxx.vercel.app`；但 **API 需要一個指向 VM 的網域**才能讓 Caddy 自動申請 HTTPS 憑證（可用免費的 DuckDNS 等動態 DNS）。

---

## 0. 需要準備的帳號與資料

| 項目 | 取得方式 | 放在哪裡 |
|---|---|---|
| Neon 正式 branch 連線字串 | Neon 後台 → Branches → 建 `production` → Connect | backend env `DATABASE_URL` |
| LINE Login channel | LINE Developers → 建立 Provider → LINE Login channel | `LINE_CHANNEL_ID` / `LINE_CHANNEL_SECRET` |
| 綠界正式商店 | 綠界商店後台 → 系統開發管理 → 系統介接設定 | `ECPAY_MERCHANT_ID` / `HASH_KEY` / `HASH_IV` |
| R2 bucket 與 API token | Cloudflare → R2 → 建立 bucket、Manage API Tokens（Object Read & Write，限定此 bucket） | `R2_*` |
| VAPID 金鑰 | 本機 `cd backend && go run ./cmd/vapidkeys` | `VAPID_*` |

**所有金鑰只放在 VM 的 `/etc/critter-bid/backend.env`，不要放進 git、不要放進 Vercel。**

---

## 1. 建立 Oracle VM

1. Oracle Cloud → Compute → Instances → Create：
   * Image：Ubuntu 24.04（Canonical）
   * Shape：`VM.Standard.E2.1.Micro`（AMD）或 `VM.Standard.A1.Flex`（ARM，免費額度更大）
   * 下載 SSH 私鑰
2. VCN → Security List → Ingress 新增 TCP **80**、**443**（來源 0.0.0.0/0）。
3. DNS：`api.example.com` 的 A record 指向 VM 公開 IP。

## 2. 主機初始設定（SSH 進 VM）

```bash
sudo apt update && sudo apt -y upgrade

# 1 GB RAM：加 2 GB swap，避免偶發記憶體尖峰被 OOM kill
sudo fallocate -l 2G /swapfile && sudo chmod 600 /swapfile
sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab

# Oracle 的 Ubuntu 映像預設 iptables 會擋 80/443
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save

# 執行身分與目錄
sudo useradd --system --home /var/lib/critter-bid --shell /usr/sbin/nologin critterbid
sudo mkdir -p /opt/critter-bid /var/lib/critter-bid /etc/critter-bid
sudo chown critterbid:critterbid /var/lib/critter-bid
```

## 3. 安裝 Caddy（自動 HTTPS 反向代理）

```bash
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update && sudo apt install -y caddy
```

把本專案 `deploy/Caddyfile` 複製成 `/etc/caddy/Caddyfile`，將 `api.example.com` 改成你的網域：

```bash
sudo mkdir -p /var/log/caddy && sudo chown caddy:caddy /var/log/caddy
sudo systemctl reload caddy
```

## 4. 在本機 build 後端

需要 **Go 1.27.3 以上**（1.27.0～1.27.2 有已知的 net/http、crypto 漏洞，build script 會拒絕）：

```bash
scripts/build-backend.sh amd64    # E2 Micro（AMD）
scripts/build-backend.sh arm64    # A1（ARM）
```

產出 `dist/critter-bid-linux-<arch>`（單一靜態執行檔，已內含資料庫 migration）。

## 5. 上傳並設定

```bash
scp dist/critter-bid-linux-amd64 ubuntu@api.example.com:/tmp/critter-bid
ssh ubuntu@api.example.com
sudo install -m 755 /tmp/critter-bid /opt/critter-bid/critter-bid
```

建立設定檔（以 `deploy/backend.env.production.example` 為範本填入真實值）：

```bash
sudo nano /etc/critter-bid/backend.env
sudo chown root:critterbid /etc/critter-bid/backend.env
sudo chmod 640 /etc/critter-bid/backend.env
```

安裝 systemd 服務（`deploy/critter-bid.service`）：

```bash
sudo cp critter-bid.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now critter-bid
sudo systemctl status critter-bid
journalctl -u critter-bid -f            # 看 log（JSON 格式）
curl https://api.example.com/api/health # {"status":"ok"}
```

啟動時會自動套用資料庫 migration（會自動改連 Neon 的 direct host）。設定有錯時程式會列出所有錯誤並拒絕啟動，例如正式環境開了 `MOCK_LINE_AUTH`。

## 6. Cloudflare R2

1. 建立 bucket（例如 `critter-bid-images`）。
2. Settings → Public access：連接自訂網域 `img.example.com`（或啟用 r2.dev 網址）。
3. Manage R2 API Tokens → 建立 **Object Read & Write**、只限此 bucket 的 token，填入 `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY`；`R2_ACCOUNT_ID` 在 R2 首頁右側。
4. backend env：`STORAGE_BACKEND=r2`、`R2_PUBLIC_BASE_URL=https://img.example.com`。

## 7. Vercel（前端）

1. Import 這個 Git repo，**Root Directory 設為 `frontend`**。
2. Environment Variables：
   * `BACKEND_URL=https://api.example.com`
   * `NEXT_PUBLIC_IMAGE_ORIGIN=https://img.example.com`（讓 CSP 允許 R2 圖片）
   * **不要**設定 `NEXT_PUBLIC_MOCK_LOGIN`
3. Domains：綁定 `app.example.com`。

## 7.5 初期測試：測試登入（不需要 LINE）

在 `/etc/critter-bid/backend.env` 設定 `TEST_LOGIN_PASSWORD`（至少 16 字元，例如用 `openssl rand -base64 24` 產生）與 `ADMIN_LINE_USER_IDS=test-admin`，重啟服務後：

* 登入頁會出現「測試登入」：輸入任意名稱（如 `admin`、`seller`、`buyer1`、`buyer2`）＋這組密碼，每個名稱都是獨立帳號。
* `admin` 為管理員。可用不同瀏覽器或無痕視窗同時登入多個帳號一起競標。
* 正式開放前**刪除 `TEST_LOGIN_PASSWORD`** 並重啟即可關閉；LINE 設定好之前可以只用測試登入。

## 8. 外部服務設定

* **LINE Developers** → LINE Login channel → Callback URL：`https://app.example.com/api/auth/line/callback`；channel 狀態改為 Published。
* **管理員**：先用自己的 LINE 登入一次，在 Neon SQL Editor 執行 `SELECT line_user_id, display_name FROM users;` 取得你的 `line_user_id`，填入 `ADMIN_LINE_USER_IDS` 後 `sudo systemctl restart critter-bid`，重新登入即為管理員。
* **綠界**：付款通知網址由程式自動帶入（`PUBLIC_BASE_URL/api/payments/ecpay/notify`），不需在後台設定。上線前建議先用 stage 測試商店（`.env.example` 內的公開測試帳號）＋ 綠界測試卡完整走一次付款，確認 callback 能抵達。
* **Client IP**：部署後到「管理後台 → 稽核紀錄」看你自己登入的 IP。若顯示的是 Vercel 或 Caddy 的 IP，把 `CLIENT_IP_HEADER` 改成 `X-Vercel-Forwarded-For` 或 `X-Forwarded-For` 再測。

## 9. 更新版本

```bash
scripts/build-backend.sh amd64
scp dist/critter-bid-linux-amd64 ubuntu@api.example.com:/tmp/critter-bid
ssh ubuntu@api.example.com 'sudo install -m 755 /tmp/critter-bid /opt/critter-bid/critter-bid && sudo systemctl restart critter-bid'
```

服務收到 SIGTERM 會優雅關閉：處理完進行中的請求、等待 scheduler 該輪結束與在途推播。前端推到 Git 後由 Vercel 自動部署。

## 10. 資源用量與調整

* systemd 設定 `GOMEMLIMIT=300MiB`、`MemoryMax=450M`。正常閒置約數十 MB；處理圖片時一次只解碼一張，以限制記憶體峰值。
* VM 太吃力時，可把 `SCHEDULER_INTERVAL` 調為 `5m` 或 `10m`。所有定時工作都是冪等的，只會讓結標與通知稍晚。
* 資料庫連線池最多 10 條、閒置 5 分鐘釋放，Neon 可以自動休眠。

## 11. 備份

* Neon：Free 方案有 point-in-time restore（保留期依方案）；重要時刻可在 Neon 建立 branch 作為快照。
* R2：圖片 key 都由伺服器產生且不覆寫；如需備份可用 `rclone` 同步 bucket。
* `audit_logs` 只能新增、不能修改或刪除（資料庫 trigger）。
