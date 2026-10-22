package admin

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/auction"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/testdb"
)

// Integration tests: need TEST_DATABASE_URL.

type env struct {
	pool  *pgxpool.Pool
	st    *store.Store
	svc   *Service
	admin uuid.UUID
	now   time.Time
}

func setup(t *testing.T) env {
	pool := testdb.New(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	e := env{pool: pool, st: store.New(pool), now: now}
	e.svc = NewService(e.st, notify.Noop{}, func() time.Time { return now })
	e.admin = testdb.CreateUser(t, pool, "admin")
	if _, err := pool.Exec(context.Background(), "UPDATE users SET role='ADMIN' WHERE id=$1", e.admin); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegrationBanRevokesSessionsAndKeepsHistory(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	user := testdb.CreateUser(t, e.pool, "user")
	if _, err := e.pool.Exec(ctx, "INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('\\x01', $1, NOW() + INTERVAL '1 day')", user); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Ban(ctx, e.admin, user, " "); !apperr.IsCode(err, "INVALID_REASON") {
		t.Fatalf("empty reason: %v", err)
	}
	if err := e.svc.Ban(ctx, e.admin, e.admin, "x"); err != ErrCannotBanAdmin {
		t.Fatalf("self ban: %v", err)
	}
	if err := e.svc.Ban(ctx, e.admin, user, "詐騙"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Ban(ctx, e.admin, user, "again"); err != ErrAlreadyBanned {
		t.Fatalf("double ban: %v", err)
	}
	if e.count(t, "SELECT COUNT(*) FROM users WHERE id=$1 AND status='BANNED'", user) != 1 {
		t.Fatal("status not BANNED")
	}
	if e.count(t, "SELECT COUNT(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL", user) != 0 {
		t.Fatal("sessions must be revoked immediately")
	}

	if err := e.svc.Unban(ctx, e.admin, user, "誤判"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Unban(ctx, e.admin, user, "x"); err != ErrNotBanned {
		t.Fatalf("double unban: %v", err)
	}
	if err := e.svc.Ban(ctx, e.admin, user, "再犯"); err != nil {
		t.Fatal(err)
	}
	bans, err := e.st.ListUserBans(ctx, user)
	if err != nil || len(bans) != 2 {
		t.Fatalf("ban history %d err %v", len(bans), err)
	}
	if bans[1].UnbannedAt == nil || bans[1].UnbanReason != "誤判" || bans[1].BannedByName != "admin" || bans[0].UnbannedAt != nil {
		t.Fatalf("history %+v", bans)
	}
	if e.count(t, "SELECT COUNT(*) FROM audit_logs WHERE action IN ('USER_BAN','USER_UNBAN') AND actor_user_id=$1", e.admin) != 3 {
		t.Fatal("ban/unban must be audited with the admin as actor")
	}
	logs, err := e.st.AdminListAuditLogs(ctx, db.AdminListAuditLogsParams{TargetID: user.String(), Lim: 10})
	if err != nil || len(logs) != 3 || logs[0].Action != "USER_BAN" || logs[1].Action != "USER_UNBAN" || logs[0].ActorName != "admin" {
		t.Fatalf("audit search (newest first) %+v err %v", logs, err)
	}
}

func insertAuction(t *testing.T, e env, seller uuid.UUID, start, end time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO auctions (seller_id, title, description, start_price, min_increment, max_increment,
		current_price, start_time, end_time, original_end_time, shipping_method, shipping_fee)
		VALUES ($1, '守宮', '', 0, 50, 50, 0, $2, $3, $3, 'post', 0) RETURNING id`, seller, start, end).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIntegrationCloseAuctionHidesAndBlocksBids(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	seller := testdb.CreateUser(t, e.pool, "seller")
	bidder := testdb.CreateUser(t, e.pool, "bidder")
	id := insertAuction(t, e, seller, e.now.Add(-time.Hour), e.now.Add(time.Hour))
	auctions := auction.NewService(e.st, &storage.Local{BaseURL: "/uploads"}, notify.Noop{}, func() time.Time { return e.now })
	if _, err := auctions.PlaceBid(ctx, bidder, id, 50); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.CloseAuction(ctx, e.admin, id, "違禁品"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CloseAuction(ctx, e.admin, id, "again"); err != ErrAlreadyClosed {
		t.Fatalf("double close: %v", err)
	}
	other := testdb.CreateUser(t, e.pool, "other")
	if _, err := auctions.PlaceBid(ctx, other, id, 100); !apperr.IsCode(err, "AUCTION_ENDED") {
		t.Fatalf("bid on closed auction: %v", err)
	}
	d, _, err := auctions.Detail(ctx, id, uuid.Nil)
	if err != nil || d.Status != auction.StatusClosed {
		t.Fatalf("detail status %s err %v", d.Status, err)
	}
	items, _, _ := auctions.Search(ctx, "", "", 1)
	if len(items) != 0 {
		t.Fatal("closed auction must not appear on the home page")
	}
	prices, _, _ := auctions.Prices(ctx, []uuid.UUID{id})
	if len(prices) != 0 {
		t.Fatal("closed auction must drop out of price polling")
	}
	if e.count(t, "SELECT COUNT(*) FROM notifications WHERE type='AUCTION_CLOSED'") != 2 {
		t.Fatal("seller and bidder should be notified")
	}
	// Closed auctions never settle into orders.
	if e.count(t, "SELECT COUNT(*) FROM auctions WHERE id=$1 AND settled_at IS NOT NULL", id) != 1 {
		t.Fatal("closing must mark the auction settled")
	}

	settled := insertAuction(t, e, seller, e.now.Add(-3*time.Hour), e.now.Add(-time.Hour))
	if _, err := e.pool.Exec(ctx, "UPDATE auctions SET settled_at = NOW() WHERE id=$1", settled); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CloseAuction(ctx, e.admin, settled, "x"); err != ErrAlreadySettled {
		t.Fatalf("closing settled auction: %v", err)
	}
}

func TestIntegrationCancelOrderOnlyBeforePayment(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	buyer, seller := testdb.CreateUser(t, e.pool, "b"), testdb.CreateUser(t, e.pool, "s")
	mk := func(status string) uuid.UUID {
		var id uuid.UUID
		if err := e.pool.QueryRow(ctx, `INSERT INTO orders (buyer_id, seller_id, items_total, shipping_fee, total_amount, status, contact_deadline)
			VALUES ($1, $2, 100, 0, 100, $3, NOW()) RETURNING id`, buyer, seller, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	unpaid, paid := mk("AWAITING_CONTACT"), mk("AWAITING_SHIPMENT")
	if err := e.svc.CancelOrder(ctx, e.admin, unpaid, "買家逾期未聯絡"); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "SELECT COUNT(*) FROM orders WHERE id=$1 AND status='CANCELLED'", unpaid) != 1 {
		t.Fatal("not cancelled")
	}
	if err := e.svc.CancelOrder(ctx, e.admin, paid, "x"); err != ErrNotCancellable {
		t.Fatalf("cancel paid order: %v", err)
	}
}

func TestIntegrationReportHandlingNotifiesReporter(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	reporter, target := testdb.CreateUser(t, e.pool, "reporter"), testdb.CreateUser(t, e.pool, "target")
	var id, systemID uuid.UUID
	if err := e.pool.QueryRow(ctx, `INSERT INTO reports (reporter_id, target_type, target_user_id, reason)
		VALUES ($1, 'USER', $2, 'FRAUD') RETURNING id`, reporter, target).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(ctx, `INSERT INTO reports (target_type, target_user_id, reason)
		VALUES ('USER', $1, 'NO_CONTACT_24H') RETURNING id`, target).Scan(&systemID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.UpdateReport(ctx, e.admin, id, "DONE", ""); err != ErrInvalidReportOp {
		t.Fatalf("bad status: %v", err)
	}
	if err := e.svc.UpdateReport(ctx, e.admin, id, "REVIEWING", "調查中"); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "SELECT COUNT(*) FROM notifications WHERE user_id=$1", reporter) != 0 {
		t.Fatal("no notification while reviewing")
	}
	if err := e.svc.UpdateReport(ctx, e.admin, id, "RESOLVED", "已停權"); err != nil {
		t.Fatal(err)
	}
	if e.count(t, "SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND type='REPORT_RESOLVED'", reporter) != 1 {
		t.Fatal("reporter should be told the result")
	}
	if err := e.svc.UpdateReport(ctx, e.admin, id, "REJECTED", ""); !apperr.IsCode(err, "REPORT_CLOSED") {
		t.Fatalf("reopen closed report: %v", err)
	}
	if err := e.svc.UpdateReport(ctx, e.admin, systemID, "RESOLVED", "已取消訂單"); err != nil {
		t.Fatal(err)
	}
	rows, err := e.st.AdminListReports(ctx, adminListAll())
	if err != nil || len(rows) != 2 {
		t.Fatalf("admin list %d err %v", len(rows), err)
	}
}

func adminListAll() db.AdminListReportsParams { return db.AdminListReportsParams{Lim: 30} }
