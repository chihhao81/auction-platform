package auction

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/store"
)

// memStorage is an in-memory storage.Storage.
type memStorage struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMemStorage() *memStorage { return &memStorage{files: map[string][]byte{}} }

func (m *memStorage) Put(_ context.Context, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[key] = data
	return nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	return nil
}

func (m *memStorage) URL(key string) string { return "/uploads/" + key }

func (m *memStorage) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.files)
}

// recorder is a notify.Dispatcher that remembers what was dispatched.
type recorder struct {
	mu   sync.Mutex
	sent []db.Notification
}

func (r *recorder) Dispatch(n []db.Notification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, n...)
}

func newTestService(t *testing.T, now time.Time) (*Service, pgxmock.PgxPoolIface, *memStorage) {
	svc, mock, files, _ := newTestServiceWithRecorder(t, now)
	return svc, mock, files
}

func newTestServiceWithRecorder(t *testing.T, now time.Time) (*Service, pgxmock.PgxPoolIface, *memStorage, *recorder) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	files := newMemStorage()
	rec := &recorder{}
	return NewService(store.New(mock), files, rec, func() time.Time { return now }), mock, files, rec
}

var auctionCols = []string{
	"id", "seller_id", "title", "description", "start_price", "min_increment", "max_increment",
	"current_price", "highest_bidder_id", "bid_count", "start_time", "end_time", "original_end_time",
	"contact_info", "shipping_method", "shipping_fee", "created_at", "updated_at",
	"settled_at", "ending_soon_notified_at",
}

func auctionRows(a db.Auction) *pgxmock.Rows {
	return pgxmock.NewRows(auctionCols).AddRow(
		a.ID, a.SellerID, a.Title, a.Description, a.StartPrice, a.MinIncrement, a.MaxIncrement,
		a.CurrentPrice, a.HighestBidderID, a.BidCount, a.StartTime, a.EndTime, a.OriginalEndTime,
		a.ContactInfo, a.ShippingMethod, a.ShippingFee, a.CreatedAt, a.UpdatedAt,
		a.SettledAt, a.EndingSoonNotifiedAt,
	)
}

var bidCols = []string{"id", "auction_id", "bidder_id", "amount", "created_at"}

func expectAudit(mock pgxmock.PgxPoolIface, action string) {
	mock.ExpectExec("INSERT INTO audit_logs").
		WithArgs(pgxmock.AnyArg(), action, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
}

var notificationCols = []string{"id", "user_id", "type", "title", "content", "related_auction_id", "related_order_id", "read_at", "created_at"}

func TestPlaceBidSuccessNotifiesOutbidUser(t *testing.T) {
	svc, mock, _, rec := newTestServiceWithRecorder(t, t0)
	a := activeAuction()
	a.HighestBidderID, a.BidCount = uuid.NullUUID{UUID: bob, Valid: true}, 1
	updated := a
	updated.CurrentPrice, updated.HighestBidderID, updated.BidCount = 1200, uuid.NullUUID{UUID: alice, Valid: true}, 2
	bidID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM auctions WHERE id = \$1 FOR UPDATE`).WithArgs(a.ID).WillReturnRows(auctionRows(a))
	mock.ExpectQuery("INSERT INTO bids").WithArgs(a.ID, alice, int64(1200), t0).
		WillReturnRows(pgxmock.NewRows(bidCols).AddRow(bidID, a.ID, alice, int64(1200), t0))
	mock.ExpectQuery("UPDATE auctions").WithArgs(a.ID, int64(1200), uuid.NullUUID{UUID: alice, Valid: true}, a.EndTime, t0).
		WillReturnRows(auctionRows(updated))
	expectAudit(mock, "BID_PLACE")
	mock.ExpectQuery("INSERT INTO notifications").
		WithArgs(bob, "OUTBID", pgxmock.AnyArg(), pgxmock.AnyArg(), uuid.NullUUID{UUID: a.ID, Valid: true}, uuid.NullUUID{}, t0).
		WillReturnRows(pgxmock.NewRows(notificationCols).AddRow(uuid.New(), bob, "OUTBID", "t", "c", uuid.NullUUID{UUID: a.ID, Valid: true}, uuid.NullUUID{}, (*time.Time)(nil), t0))
	mock.ExpectCommit()

	res, err := svc.PlaceBid(context.Background(), alice, a.ID, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if res.Extended || res.Auction.CurrentPrice != 1200 || res.PreviousBidder.UUID != bob {
		t.Fatalf("unexpected result %+v", res)
	}
	if len(rec.sent) != 1 || rec.sent[0].UserID != bob {
		t.Fatalf("outbid notification should be dispatched after commit: %+v", rec.sent)
	}
}

func TestPlaceBidExtendsInLastMinute(t *testing.T) {
	a := activeAuction()
	now := a.EndTime.Add(-40 * time.Second)
	svc, mock, _ := newTestService(t, now)
	newEnd := a.EndTime.Add(2 * time.Minute)
	updated := a
	updated.CurrentPrice, updated.EndTime, updated.BidCount = 1050, newEnd, 1
	updated.HighestBidderID = uuid.NullUUID{UUID: alice, Valid: true}

	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs(a.ID).WillReturnRows(auctionRows(a))
	mock.ExpectQuery("INSERT INTO bids").WithArgs(a.ID, alice, int64(1050), now).
		WillReturnRows(pgxmock.NewRows(bidCols).AddRow(uuid.New(), a.ID, alice, int64(1050), now))
	mock.ExpectQuery("UPDATE auctions").WithArgs(a.ID, int64(1050), pgxmock.AnyArg(), newEnd, now).
		WillReturnRows(auctionRows(updated))
	expectAudit(mock, "BID_PLACE")
	mock.ExpectCommit()

	res, err := svc.PlaceBid(context.Background(), alice, a.ID, 1050)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Extended || !res.Auction.EndTime.Equal(newEnd) {
		t.Fatalf("expected extension to %v, got %+v", newEnd, res)
	}
}

func TestPlaceBidInvalidRollsBackWithoutWrites(t *testing.T) {
	svc, mock, _ := newTestService(t, t0)
	a := activeAuction()
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs(a.ID).WillReturnRows(auctionRows(a))
	mock.ExpectRollback()

	_, err := svc.PlaceBid(context.Background(), seller, a.ID, 1100)
	if !apperr.IsCode(err, "SELLER_CANNOT_BID") {
		t.Fatalf("got %v", err)
	}
}

func TestPlaceBidAfterEndRejected(t *testing.T) {
	a := activeAuction()
	svc, mock, _ := newTestService(t, a.EndTime.Add(time.Second))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs(a.ID).WillReturnRows(auctionRows(a))
	mock.ExpectRollback()
	if _, err := svc.PlaceBid(context.Background(), alice, a.ID, 1100); !apperr.IsCode(err, "AUCTION_ENDED") {
		t.Fatalf("got %v", err)
	}
}

func TestPlaceBidDuplicateAmountMapsToConflict(t *testing.T) {
	svc, mock, _ := newTestService(t, t0)
	a := activeAuction()
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs(a.ID).WillReturnRows(auctionRows(a))
	mock.ExpectQuery("INSERT INTO bids").WithArgs(a.ID, alice, int64(1100), t0).WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()
	if _, err := svc.PlaceBid(context.Background(), alice, a.ID, 1100); !apperr.IsCode(err, "BID_CONFLICT") {
		t.Fatalf("got %v", err)
	}
}

func TestPlaceBidNotFound(t *testing.T) {
	svc, mock, _ := newTestService(t, t0)
	id := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs(id).WillReturnRows(pgxmock.NewRows(auctionCols))
	mock.ExpectRollback()
	if _, err := svc.PlaceBid(context.Background(), alice, id, 1100); !apperr.IsCode(err, "AUCTION_NOT_FOUND") {
		t.Fatalf("got %v", err)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.NRGBA{R: 255, A: 128})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func validInput() Input {
	return Input{Title: "守宮", ShippingMethod: "黑貓", StartTime: ptr(t0.Add(time.Hour)), EndTime: ptr(t0.Add(49 * time.Hour))}
}

func TestCreateRejectsWhenSellerHasFiveOpenAuctions(t *testing.T) {
	svc, mock, files := newTestService(t, t0)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM users").WithArgs(seller).WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(seller, t0).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(5)))
	mock.ExpectRollback()

	_, err := svc.Create(context.Background(), seller, validInput(), []Upload{{Filename: "a.png", Data: pngBytes(t)}})
	if !apperr.IsCode(err, "TOO_MANY_OPEN_AUCTIONS") {
		t.Fatalf("got %v", err)
	}
	if files.count() != 0 {
		t.Fatalf("uploaded files must be cleaned up, %d left", files.count())
	}
}

func TestCreateRejectsBannedSeller(t *testing.T) {
	svc, mock, files := newTestService(t, t0)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM users").WithArgs(seller).WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("BANNED"))
	mock.ExpectRollback()
	_, err := svc.Create(context.Background(), seller, validInput(), []Upload{{Filename: "a.png", Data: pngBytes(t)}})
	if !apperr.IsCode(err, "USER_BANNED") || files.count() != 0 {
		t.Fatalf("got %v, files %d", err, files.count())
	}
}

func TestCreateSuccess(t *testing.T) {
	svc, mock, files := newTestService(t, t0)
	in := validInput()
	created := db.Auction{ID: uuid.New(), SellerID: seller, Title: in.Title, StartTime: *in.StartTime, EndTime: *in.EndTime}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM users").WithArgs(seller).WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("ACTIVE"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(seller, t0).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(4)))
	mock.ExpectQuery("INSERT INTO auctions").
		WithArgs(seller, "守宮", "", int64(0), int64(50), int64(50), *in.StartTime, *in.EndTime, "", "黑貓", int64(0), t0).
		WillReturnRows(auctionRows(created))
	mock.ExpectExec("INSERT INTO auction_images").WithArgs(created.ID, pgxmock.AnyArg(), int16(0), int32(4), int32(3), t0).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	expectAudit(mock, "AUCTION_CREATE")
	mock.ExpectCommit()

	got, err := svc.Create(context.Background(), seller, in, []Upload{{Filename: "a.png", Data: pngBytes(t)}})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || files.count() != 1 {
		t.Fatalf("got %+v, files %d", got, files.count())
	}
}

func TestCreateRequiresImages(t *testing.T) {
	svc, _, _ := newTestService(t, t0)
	if _, err := svc.Create(context.Background(), seller, validInput(), nil); !apperr.IsCode(err, "INVALID_IMAGE_COUNT") {
		t.Fatalf("got %v", err)
	}
	five := make([]Upload, 5)
	if _, err := svc.Create(context.Background(), seller, validInput(), five); !apperr.IsCode(err, "INVALID_IMAGE_COUNT") {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateRules(t *testing.T) {
	upcoming := activeAuction()
	upcoming.StartTime, upcoming.EndTime = t0.Add(time.Hour), t0.Add(3*time.Hour)

	cases := []struct {
		name string
		a    db.Auction
		user uuid.UUID
		code string
	}{
		{"not the seller", upcoming, alice, "NOT_SELLER"},
		{"already active", activeAuction(), seller, "AUCTION_NOT_EDITABLE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock, files := newTestService(t, t0)
			mock.ExpectBegin()
			mock.ExpectQuery("FOR UPDATE").WithArgs(tc.a.ID).WillReturnRows(auctionRows(tc.a))
			mock.ExpectRollback()
			_, err := svc.Update(context.Background(), tc.user, tc.a.ID, validInput(), nil, []Upload{{Filename: "a.png", Data: pngBytes(t)}})
			if !apperr.IsCode(err, tc.code) {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
			if files.count() != 0 {
				t.Fatal("new uploads must be removed when the update fails")
			}
		})
	}
}

func TestUpdateRejectsForeignImageIDs(t *testing.T) {
	upcoming := activeAuction()
	upcoming.StartTime, upcoming.EndTime = t0.Add(time.Hour), t0.Add(3*time.Hour)
	svc, mock, _ := newTestService(t, t0)
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs(upcoming.ID).WillReturnRows(auctionRows(upcoming))
	mock.ExpectQuery("FROM auction_images").WithArgs([]uuid.UUID{upcoming.ID}).
		WillReturnRows(pgxmock.NewRows([]string{"id", "auction_id", "storage_key", "position", "width", "height", "created_at"}))
	mock.ExpectRollback()
	_, err := svc.Update(context.Background(), seller, upcoming.ID, validInput(), []uuid.UUID{uuid.New()}, nil)
	if !apperr.IsCode(err, "UNKNOWN_IMAGE") {
		t.Fatalf("got %v", err)
	}
}
