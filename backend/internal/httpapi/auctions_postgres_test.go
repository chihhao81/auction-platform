package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// This integration test only runs against an explicitly provided test database.
// It creates and drops a uniquely named schema; do not point the variable at a
// production database.
func TestConcurrentBidsAreSerializedByPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("AUCTION_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set AUCTION_TEST_DATABASE_URL to a disposable PostgreSQL database to run concurrency integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "auction_test_" + hex.EncodeToString(random[:])
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	admin := stdlib.OpenDB(*adminConfig)
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()

	testConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	testConfig.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*testConfig)
	defer db.Close()
	db.SetMaxOpenConns(8)
	ddls := []string{
		`CREATE TABLE users (id BIGINT PRIMARY KEY, status TEXT NOT NULL)`,
		`CREATE TABLE user_terms (user_id BIGINT NOT NULL, terms_version TEXT NOT NULL, PRIMARY KEY(user_id,terms_version))`,
		`CREATE TABLE auctions (
			id BIGINT PRIMARY KEY, seller_id BIGINT NOT NULL, current_price BIGINT NOT NULL,
			min_increment BIGINT NOT NULL, max_increment BIGINT NOT NULL,
			starts_at TIMESTAMPTZ NOT NULL, ends_at TIMESTAMPTZ NOT NULL,
			highest_bidder_id BIGINT, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE bids (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, auction_id BIGINT NOT NULL,
			bidder_id BIGINT NOT NULL, amount BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	}
	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("create isolated test table: %v", err)
		}
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,status) VALUES (1,'ACTIVE'),(2,'ACTIVE'),(3,'ACTIVE')`); err != nil {
		t.Fatalf("seed test users: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_terms(user_id,terms_version) VALUES (1,'v1'),(2,'v1'),(3,'v1')`); err != nil {
		t.Fatalf("seed test terms: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO auctions(id,seller_id,current_price,min_increment,max_increment,starts_at,ends_at) VALUES (1,1,1000,50,500,$1,$2)`, now.Add(-time.Hour), now.Add(time.Minute)); err != nil {
		t.Fatalf("seed test auction: %v", err)
	}

	cfg := Config{FrontendURL: "https://auction.test", SessionSecret: "0123456789abcdef0123456789abcdef", TermsVersion: "v1", CookieSameSite: "lax"}
	app := &App{db: db, cfg: cfg}
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, bidderID := range []int64{2, 3} {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			<-start
			request := httptest.NewRequest(http.MethodPost, "/v1/auctions/1/bids", strings.NewReader(`{"amount":1050}`))
			request.SetPathValue("id", "1")
			request.Header.Set("Origin", cfg.FrontendURL)
			cookieResponse := httptest.NewRecorder()
			app.setSession(cookieResponse, userID)
			request.AddCookie(cookieResponse.Result().Cookies()[0])
			response := httptest.NewRecorder()
			app.placeBid(response, request)
			statuses <- response.Code
		}(bidderID)
	}
	close(start)
	wg.Wait()
	close(statuses)
	successes, rejections := 0, 0
	for status := range statuses {
		switch status {
		case http.StatusCreated:
			successes++
		case http.StatusBadRequest:
			rejections++
		default:
			t.Errorf("unexpected concurrent bid response: %d", status)
		}
	}
	if successes != 1 || rejections != 1 {
		t.Fatalf("got %d accepted and %d rejected bids, want exactly one each", successes, rejections)
	}
	var bidCount int
	var current int64
	var winner sql.NullInt64
	var end time.Time
	if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM bids),current_price,highest_bidder_id,ends_at FROM auctions WHERE id=1`).Scan(&bidCount, &current, &winner, &end); err != nil {
		t.Fatal(err)
	}
	if bidCount != 1 || current != 1050 || !winner.Valid || (winner.Int64 != 2 && winner.Int64 != 3) {
		t.Fatalf("inconsistent result: bids=%d current=%d winner=%+v", bidCount, current, winner)
	}
	if !end.After(now.Add(time.Minute)) {
		t.Fatalf("last-minute bid did not extend auction: ends_at=%s", end)
	}
}
