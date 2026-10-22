package auction

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/imageproc"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/storage"
	"auction-platform/backend/internal/store"
)

var (
	ErrNotFound        = apperr.NotFound("AUCTION_NOT_FOUND", "找不到此競標")
	ErrNotSeller       = apperr.Forbidden("NOT_SELLER", "只有賣家可以修改此競標")
	ErrNotEditable     = apperr.Conflict("AUCTION_NOT_EDITABLE", "只有尚未開始的競標可以修改")
	ErrTooManyOpen     = apperr.Conflict("TOO_MANY_OPEN_AUCTIONS", fmt.Sprintf("同時最多只能有 %d 筆即將開始或進行中的競標", MaxOpenAuctionsPerSeller))
	ErrImageCount      = apperr.Invalid("INVALID_IMAGE_COUNT", fmt.Sprintf("商品圖片需為 %d ~ %d 張", MinImages, MaxImages))
	ErrUnknownImage    = apperr.Invalid("UNKNOWN_IMAGE", "保留的圖片不屬於此競標")
	ErrBidConflict     = apperr.Conflict("BID_CONFLICT", "已有其他使用者以相同金額出價，請重新整理後再試")
	ErrUserNotActive   = apperr.Forbidden("USER_BANNED", "此帳號已被停權")
	errStatusNotActive = errors.New("user not active")
)

type Service struct {
	store    *store.Store
	files    storage.Storage
	dispatch notify.Dispatcher
	now      func() time.Time
}

func NewService(st *store.Store, files storage.Storage, d notify.Dispatcher, now func() time.Time) *Service {
	return &Service{store: st, files: files, dispatch: d, now: now}
}

// Upload is one uploaded image file. Filename is only used for its extension.
type Upload struct {
	Filename string
	Data     []byte
}

type storedImage struct {
	Key           string
	Width, Height int
}

// storeUploads validates, re-encodes and stores uploads. On error, anything
// already stored is removed again.
func (s *Service) storeUploads(ctx context.Context, uploads []Upload) ([]storedImage, error) {
	out := make([]storedImage, 0, len(uploads))
	for _, u := range uploads {
		res, err := imageproc.Process(u.Data, u.Filename)
		if err == nil {
			key := storage.NewImageKey("auctions")
			if err = s.files.Put(ctx, key, res.Data, "image/jpeg"); err == nil {
				out = append(out, storedImage{Key: key, Width: res.Width, Height: res.Height})
				continue
			}
		}
		s.deleteFiles(ctx, keysOf(out))
		return nil, err
	}
	return out, nil
}

func keysOf(imgs []storedImage) []string {
	keys := make([]string, len(imgs))
	for i, img := range imgs {
		keys[i] = img.Key
	}
	return keys
}

// deleteFiles removes stored objects on a best-effort basis. Failures only
// leave orphaned files behind, so they are logged rather than returned.
func (s *Service) deleteFiles(ctx context.Context, keys []string) {
	for _, k := range keys {
		if err := s.files.Delete(ctx, k); err != nil {
			slog.WarnContext(ctx, "delete stored file", "key", k, "err", err)
		}
	}
}

func insertImages(ctx context.Context, q *db.Queries, auctionID uuid.UUID, imgs []storedImage, now time.Time) error {
	for i, img := range imgs {
		if err := q.InsertAuctionImage(ctx, db.InsertAuctionImageParams{
			AuctionID: auctionID, StorageKey: img.Key, Position: int16(i),
			Width: int32(img.Width), Height: int32(img.Height), CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("insert image: %w", err)
		}
	}
	return nil
}

// Create publishes a new auction for seller.
func (s *Service) Create(ctx context.Context, seller uuid.UUID, in Input, uploads []Upload) (db.Auction, error) {
	now := s.now().UTC()
	f, err := in.Normalize(now)
	if err != nil {
		return db.Auction{}, err
	}
	if len(uploads) < MinImages || len(uploads) > MaxImages {
		return db.Auction{}, ErrImageCount
	}
	imgs, err := s.storeUploads(ctx, uploads)
	if err != nil {
		return db.Auction{}, err
	}

	var created db.Auction
	err = s.store.InTx(ctx, func(q *db.Queries) error {
		status, err := q.LockUserStatus(ctx, seller)
		if err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if status != "ACTIVE" {
			return errStatusNotActive
		}
		open, err := q.CountOpenAuctionsBySeller(ctx, db.CountOpenAuctionsBySellerParams{SellerID: seller, Now: now})
		if err != nil {
			return fmt.Errorf("count open auctions: %w", err)
		}
		if open >= MaxOpenAuctionsPerSeller {
			return ErrTooManyOpen
		}
		created, err = q.CreateAuction(ctx, db.CreateAuctionParams{
			SellerID: seller, Title: f.Title, Description: f.Description,
			StartPrice: f.StartPrice, MinIncrement: f.MinIncrement, MaxIncrement: f.MaxIncrement,
			StartTime: f.StartTime, EndTime: f.EndTime, ContactInfo: f.ContactInfo,
			ShippingMethod: f.ShippingMethod, ShippingFee: f.ShippingFee, CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("create auction: %w", err)
		}
		if err := insertImages(ctx, q, created.ID, imgs, now); err != nil {
			return err
		}
		return audit.Record(ctx, q, now, audit.Entry{
			Actor: seller, Action: audit.ActionAuctionCreate, TargetType: audit.TargetAuction, TargetID: created.ID.String(),
			Metadata: map[string]any{"title": f.Title, "start_price": f.StartPrice, "start_time": f.StartTime, "end_time": f.EndTime, "images": len(imgs)},
		})
	})
	if err != nil {
		s.deleteFiles(ctx, keysOf(imgs))
		if errors.Is(err, errStatusNotActive) {
			return db.Auction{}, ErrUserNotActive
		}
		return db.Auction{}, err
	}
	return created, nil
}

// Update edits an auction that has not started yet. keep lists the existing
// image IDs to keep, in display order; new uploads are appended after them.
func (s *Service) Update(ctx context.Context, user, id uuid.UUID, in Input, keep []uuid.UUID, uploads []Upload) (db.Auction, error) {
	now := s.now().UTC()
	f, err := in.Normalize(now)
	if err != nil {
		return db.Auction{}, err
	}
	if total := len(keep) + len(uploads); total < MinImages || total > MaxImages {
		return db.Auction{}, ErrImageCount
	}
	added, err := s.storeUploads(ctx, uploads)
	if err != nil {
		return db.Auction{}, err
	}

	var updated db.Auction
	var removed []string
	err = s.store.InTx(ctx, func(q *db.Queries) error {
		cur, err := q.GetAuctionForUpdate(ctx, id)
		if store.IsNoRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock auction: %w", err)
		}
		if cur.SellerID != user {
			return ErrNotSeller
		}
		if StatusOf(cur, now) != StatusUpcoming {
			return ErrNotEditable
		}

		existing, err := q.ListImagesForAuctions(ctx, []uuid.UUID{id})
		if err != nil {
			return fmt.Errorf("list images: %w", err)
		}
		byID := make(map[uuid.UUID]db.AuctionImage, len(existing))
		for _, img := range existing {
			byID[img.ID] = img
		}
		final := make([]storedImage, 0, len(keep)+len(added))
		kept := map[uuid.UUID]bool{}
		for _, kid := range keep {
			img, ok := byID[kid]
			if !ok || kept[kid] {
				return ErrUnknownImage
			}
			kept[kid] = true
			final = append(final, storedImage{Key: img.StorageKey, Width: int(img.Width), Height: int(img.Height)})
		}
		final = append(final, added...)
		removed = removed[:0]
		for _, img := range existing {
			if !kept[img.ID] {
				removed = append(removed, img.StorageKey)
			}
		}

		updated, err = q.UpdateAuctionContent(ctx, db.UpdateAuctionContentParams{
			ID: id, Title: f.Title, Description: f.Description, StartPrice: f.StartPrice,
			MinIncrement: f.MinIncrement, MaxIncrement: f.MaxIncrement, StartTime: f.StartTime,
			EndTime: f.EndTime, ContactInfo: f.ContactInfo, ShippingMethod: f.ShippingMethod,
			ShippingFee: f.ShippingFee, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("update auction: %w", err)
		}
		if err := q.DeleteAuctionImages(ctx, id); err != nil {
			return fmt.Errorf("delete images: %w", err)
		}
		if err := insertImages(ctx, q, id, final, now); err != nil {
			return err
		}
		return audit.Record(ctx, q, now, audit.Entry{
			Actor: user, Action: audit.ActionAuctionUpdate, TargetType: audit.TargetAuction, TargetID: id.String(),
			Metadata: map[string]any{"changes": diff(cur, updated), "images_removed": len(removed), "images_added": len(added)},
		})
	})
	if err != nil {
		s.deleteFiles(ctx, keysOf(added))
		return db.Auction{}, err
	}
	s.deleteFiles(ctx, removed)
	return updated, nil
}

// diff lists changed fields as {field: [before, after]} for the audit log.
func diff(a, b db.Auction) map[string][2]any {
	d := map[string][2]any{}
	add := func(name string, x, y any) {
		if x != y {
			d[name] = [2]any{x, y}
		}
	}
	add("title", a.Title, b.Title)
	add("description", a.Description, b.Description)
	add("start_price", a.StartPrice, b.StartPrice)
	add("min_increment", a.MinIncrement, b.MinIncrement)
	add("max_increment", a.MaxIncrement, b.MaxIncrement)
	add("start_time", a.StartTime.UTC(), b.StartTime.UTC())
	add("end_time", a.EndTime.UTC(), b.EndTime.UTC())
	add("contact_info", a.ContactInfo, b.ContactInfo)
	add("shipping_method", a.ShippingMethod, b.ShippingMethod)
	add("shipping_fee", a.ShippingFee, b.ShippingFee)
	return d
}

// BidResult describes an accepted bid.
type BidResult struct {
	Bid      db.Bid
	Auction  db.Auction
	Extended bool
	// PreviousBidder is the user who was outbid, if any (for notifications).
	PreviousBidder uuid.NullUUID
}

// PlaceBid atomically validates and applies a bid. The auction row is locked
// with SELECT ... FOR UPDATE, so concurrent bids on the same auction are
// serialised: each one is validated against the price left by the previous
// one, and the clock is read only after the lock is held.
func (s *Service) PlaceBid(ctx context.Context, bidder, auctionID uuid.UUID, amount int64) (BidResult, error) {
	var res BidResult
	var batch notify.Batch
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		a, err := q.GetAuctionForUpdate(ctx, auctionID)
		if store.IsNoRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock auction: %w", err)
		}
		now := s.now().UTC()
		if err := ValidateBid(a, bidder, amount, now); err != nil {
			return err
		}
		newEnd, extended := ExtendedEnd(a.EndTime, now)

		bid, err := q.InsertBid(ctx, db.InsertBidParams{AuctionID: a.ID, BidderID: bidder, Amount: amount, CreatedAt: now})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrBidConflict
			}
			return fmt.Errorf("insert bid: %w", err)
		}
		updated, err := q.ApplyBid(ctx, db.ApplyBidParams{
			ID: a.ID, CurrentPrice: amount, HighestBidderID: uuid.NullUUID{UUID: bidder, Valid: true},
			EndTime: newEnd, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("apply bid: %w", err)
		}
		meta := map[string]any{"amount": amount, "previous_price": a.CurrentPrice, "bid_id": bid.ID.String()}
		if extended {
			meta["extended_from"] = a.EndTime.UTC()
			meta["extended_to"] = newEnd.UTC()
		}
		if err := audit.Record(ctx, q, now, audit.Entry{
			Actor: bidder, Action: audit.ActionBidPlace, TargetType: audit.TargetAuction, TargetID: a.ID.String(), Metadata: meta,
		}); err != nil {
			return err
		}
		if a.HighestBidderID.Valid && a.HighestBidderID.UUID != bidder {
			if err := batch.Add(ctx, q, now, notify.Message{
				UserID: a.HighestBidderID.UUID, Type: notify.TypeOutbid, AuctionID: a.ID, Title: "您的出價已被超越",
				Content: fmt.Sprintf("「%s」目前價格 %s。", a.Title, notify.FormatPrice(amount)),
			}); err != nil {
				return err
			}
		}
		res = BidResult{Bid: bid, Auction: updated, Extended: extended, PreviousBidder: a.HighestBidderID}
		return nil
	})
	if err != nil {
		return BidResult{}, err
	}
	s.dispatch.Dispatch(batch)
	return res, nil
}
