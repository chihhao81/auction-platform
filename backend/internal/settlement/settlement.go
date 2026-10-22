// Package settlement holds the periodic jobs run by the scheduler: closing
// ended auctions, merging wins into orders and "ending soon" notices.
//
// Every job is idempotent and safe to run concurrently with itself (rows are
// claimed under locks or conditional updates), so a slower schedule, a missed
// tick or two server instances never produce duplicate results.
package settlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/notify"
	"auction-platform/backend/internal/store"
)

const (
	// MergeWindow: when a seller has another auction ending within this
	// window, wins wait so they can be merged into one order.
	MergeWindow = 10 * time.Minute
	// EndingSoonWindow: bidders are told an auction ends within this window.
	EndingSoonWindow = 10 * time.Minute
	// ContactDeadline: the buyer must message the seller within this time.
	ContactDeadline = 24 * time.Hour

	batchSize = 100
)

type Service struct {
	store    *store.Store
	dispatch notify.Dispatcher
	now      func() time.Time
}

func NewService(st *store.Store, d notify.Dispatcher, now func() time.Time) *Service {
	return &Service{store: st, dispatch: d, now: now}
}

// RunOnce runs every job once. Errors in one job do not stop the others.
func (s *Service) RunOnce(ctx context.Context) error {
	var errs []error
	ending, err := s.NotifyEndingSoon(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("ending soon: %w", err))
	}
	settled, err := s.SettleEnded(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("settle: %w", err))
	}
	orders, err := s.FinalizeOrders(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("finalize orders: %w", err))
	}
	if ending+settled+orders > 0 {
		slog.InfoContext(ctx, "settlement run", "ending_soon", ending, "settled", settled, "orders_created", orders)
	}
	return errors.Join(errs...)
}

// SettleEnded settles auctions whose end time has passed. It returns the
// number of auctions settled.
func (s *Service) SettleEnded(ctx context.Context) (int, error) {
	ids, err := s.store.ListUnsettledEndedAuctionIDs(ctx, db.ListUnsettledEndedAuctionIDsParams{Now: s.now().UTC(), Lim: batchSize})
	if err != nil {
		return 0, fmt.Errorf("list ended auctions: %w", err)
	}
	n := 0
	var errs []error
	for _, id := range ids {
		ok, err := s.settleOne(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("auction %s: %w", id, err))
			continue
		}
		if ok {
			n++
		}
	}
	return n, errors.Join(errs...)
}

func (s *Service) settleOne(ctx context.Context, id uuid.UUID) (bool, error) {
	var batch notify.Batch
	settled := false
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		// Locking the auction row serialises with any in-flight bid.
		a, err := q.GetAuctionForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("lock auction: %w", err)
		}
		now := s.now().UTC()
		if a.SettledAt != nil || a.EndTime.After(now) {
			return nil // already settled, or extended by a last-second bid
		}
		if err := q.MarkAuctionSettled(ctx, db.MarkAuctionSettledParams{Now: now, ID: a.ID}); err != nil {
			return fmt.Errorf("mark settled: %w", err)
		}
		settled = true

		if !a.HighestBidderID.Valid {
			if err := batch.Add(ctx, q, now, notify.Message{
				UserID: a.SellerID, Type: notify.TypeAuctionUnsold, AuctionID: a.ID,
				Title: "競標結束（無人出價）", Content: fmt.Sprintf("「%s」已結束，沒有人出價。", a.Title),
			}); err != nil {
				return err
			}
			return audit.Record(ctx, q, now, audit.Entry{
				Action: audit.ActionAuctionSettle, TargetType: audit.TargetAuction, TargetID: a.ID.String(),
				Metadata: map[string]any{"winner": nil},
			})
		}

		winner := a.HighestBidderID.UUID
		w, err := q.GetUserByID(ctx, winner)
		if err != nil {
			return fmt.Errorf("load winner: %w", err)
		}
		if w.Status == "BANNED" {
			// Banned users cannot create orders: the auction ends without one.
			if err := batch.Add(ctx, q, now, notify.Message{
				UserID: a.SellerID, Type: notify.TypeAuctionUnsold, AuctionID: a.ID, Title: "競標結束（得標者已停權）",
				Content: fmt.Sprintf("「%s」的最高出價者帳號已被停權，本次競標不成立訂單。", a.Title),
			}); err != nil {
				return err
			}
			return audit.Record(ctx, q, now, audit.Entry{
				Action: audit.ActionAuctionSettle, TargetType: audit.TargetAuction, TargetID: a.ID.String(),
				Metadata: map[string]any{"winner": winner.String(), "winner_banned": true, "amount": a.CurrentPrice},
			})
		}
		if _, err := q.InsertOrderItem(ctx, db.InsertOrderItemParams{
			AuctionID: a.ID, BuyerID: winner, SellerID: a.SellerID,
			Amount: a.CurrentPrice, ShippingFee: a.ShippingFee, WonAt: now,
		}); err != nil {
			return fmt.Errorf("insert order item: %w", err)
		}
		price := notify.FormatPrice(a.CurrentPrice)
		msgs := []notify.Message{
			{UserID: winner, Type: notify.TypeAuctionWon, AuctionID: a.ID, Title: "恭喜得標！",
				Content: fmt.Sprintf("您以 %s 得標「%s」。訂單成立後請於 24 小時內透過訂單訊息聯絡賣家。", price, a.Title)},
			{UserID: a.SellerID, Type: notify.TypeAuctionSold, AuctionID: a.ID, Title: "您的商品已結標",
				Content: fmt.Sprintf("「%s」以 %s 結標。", a.Title, price)},
		}
		bidders, err := q.ListAuctionBidderIDs(ctx, a.ID)
		if err != nil {
			return fmt.Errorf("list bidders: %w", err)
		}
		for _, b := range bidders {
			if b != winner {
				msgs = append(msgs, notify.Message{UserID: b, Type: notify.TypeAuctionLost, AuctionID: a.ID, Title: "未得標",
					Content: fmt.Sprintf("「%s」已結標，得標價 %s。", a.Title, price)})
			}
		}
		waiting, err := q.SellerHasUnsettledAuctionEndingBy(ctx, db.SellerHasUnsettledAuctionEndingByParams{
			SellerID: a.SellerID, Until: now.Add(MergeWindow),
		})
		if err != nil {
			return fmt.Errorf("check merge window: %w", err)
		}
		if waiting {
			msgs = append(msgs, notify.Message{UserID: winner, Type: notify.TypeOrderWaiting, AuctionID: a.ID, Title: "等待其他競標結束",
				Content: "賣家還有其他競標即將結束，系統會在結束後將同一賣家的得標商品合併為一筆訂單。"})
		}
		for _, m := range msgs {
			if err := batch.Add(ctx, q, now, m); err != nil {
				return err
			}
		}
		return audit.Record(ctx, q, now, audit.Entry{
			Action: audit.ActionAuctionSettle, TargetType: audit.TargetAuction, TargetID: a.ID.String(),
			Metadata: map[string]any{"winner": winner.String(), "amount": a.CurrentPrice, "waiting_for_merge": waiting},
		})
	})
	if err != nil {
		return false, err
	}
	s.dispatch.Dispatch(batch)
	return settled, nil
}

// FinalizeOrders turns pending wins into orders for every seller that has no
// unsettled auction ending within MergeWindow. Wins from the same seller to
// the same buyer become a single order. It returns the number of orders
// created.
func (s *Service) FinalizeOrders(ctx context.Context) (int, error) {
	sellers, err := s.store.ListSellersWithPendingItems(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list sellers: %w", err)
	}
	total := 0
	var errs []error
	for _, seller := range sellers {
		n, err := s.finalizeSeller(ctx, seller)
		if err != nil {
			errs = append(errs, fmt.Errorf("seller %s: %w", seller, err))
			continue
		}
		total += n
	}
	return total, errors.Join(errs...)
}

// Order totals for a group of wins. Merged items ship in one parcel, so the
// shipping fee is the highest single fee rather than the sum.
func totals(items []db.OrderItem) (itemsTotal, shipping int64) {
	for _, it := range items {
		itemsTotal += it.Amount
		shipping = max(shipping, it.ShippingFee)
	}
	return itemsTotal, shipping
}

// groupByBuyer groups items by buyer, preserving the first-seen order.
func groupByBuyer(items []db.OrderItem) [][]db.OrderItem {
	idx := map[uuid.UUID]int{}
	var groups [][]db.OrderItem
	for _, it := range items {
		i, ok := idx[it.BuyerID]
		if !ok {
			i = len(groups)
			idx[it.BuyerID] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], it)
	}
	return groups
}

func (s *Service) finalizeSeller(ctx context.Context, seller uuid.UUID) (int, error) {
	var batch notify.Batch
	created := 0
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		items, err := q.LockPendingItemsForSeller(ctx, seller)
		if err != nil {
			return fmt.Errorf("lock pending items: %w", err)
		}
		if len(items) == 0 {
			return nil // another run finalized them
		}
		now := s.now().UTC()
		// Recomputed on every run, so an auction extended into (or newly
		// ending within) the window keeps the wins waiting.
		busy, err := q.SellerHasUnsettledAuctionEndingBy(ctx, db.SellerHasUnsettledAuctionEndingByParams{
			SellerID: seller, Until: now.Add(MergeWindow),
		})
		if err != nil {
			return fmt.Errorf("check merge window: %w", err)
		}
		if busy {
			return nil
		}
		for _, group := range groupByBuyer(items) {
			itemsTotal, shipping := totals(group)
			buyer := group[0].BuyerID
			o, err := q.CreateOrder(ctx, db.CreateOrderParams{
				BuyerID: buyer, SellerID: seller, ItemsTotal: itemsTotal, ShippingFee: shipping,
				TotalAmount: itemsTotal + shipping, ContactDeadline: now.Add(ContactDeadline), CreatedAt: now,
			})
			if err != nil {
				return fmt.Errorf("create order: %w", err)
			}
			ids := make([]uuid.UUID, len(group))
			auctionIDs := make([]string, len(group))
			for i, it := range group {
				ids[i], auctionIDs[i] = it.ID, it.AuctionID.String()
			}
			if err := q.AttachItemsToOrder(ctx, db.AttachItemsToOrderParams{OrderID: uuid.NullUUID{UUID: o.ID, Valid: true}, Ids: ids}); err != nil {
				return fmt.Errorf("attach items: %w", err)
			}
			total := notify.FormatPrice(o.TotalAmount)
			for _, m := range []notify.Message{
				{UserID: buyer, Type: notify.TypeOrderCreated, OrderID: o.ID, Title: "訂單已成立",
					Content: fmt.Sprintf("共 %d 件商品，總金額 %s（含運費）。請於 24 小時內透過訂單訊息聯絡賣家。", len(group), total)},
				{UserID: seller, Type: notify.TypeOrderCreated, OrderID: o.ID, Title: "新訂單成立",
					Content: fmt.Sprintf("共 %d 件商品，總金額 %s（含運費）。", len(group), total)},
			} {
				if err := batch.Add(ctx, q, now, m); err != nil {
					return err
				}
			}
			if err := audit.Record(ctx, q, now, audit.Entry{
				Action: audit.ActionOrderCreate, TargetType: audit.TargetOrder, TargetID: o.ID.String(),
				Metadata: map[string]any{"buyer": buyer.String(), "seller": seller.String(), "auctions": auctionIDs,
					"items_total": itemsTotal, "shipping_fee": shipping},
			}); err != nil {
				return err
			}
			created++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.dispatch.Dispatch(batch)
	return created, nil
}

// NotifyEndingSoon tells every bidder of an active auction ending within
// EndingSoonWindow, once per auction.
func (s *Service) NotifyEndingSoon(ctx context.Context) (int, error) {
	var batch notify.Batch
	count := 0
	err := s.store.InTx(ctx, func(q *db.Queries) error {
		now := s.now().UTC()
		claimed, err := q.ClaimEndingSoonAuctions(ctx, db.ClaimEndingSoonAuctionsParams{Now: now, Until: now.Add(EndingSoonWindow)})
		if err != nil {
			return fmt.Errorf("claim auctions: %w", err)
		}
		for _, a := range claimed {
			bidders, err := q.ListAuctionBidderIDs(ctx, a.ID)
			if err != nil {
				return fmt.Errorf("list bidders: %w", err)
			}
			for _, b := range bidders {
				if err := batch.Add(ctx, q, now, notify.Message{
					UserID: b, Type: notify.TypeAuctionEnding, AuctionID: a.ID, Title: "競標即將結束",
					Content: fmt.Sprintf("您參與的「%s」將於 10 分鐘內結束。", a.Title),
				}); err != nil {
					return err
				}
			}
		}
		count = len(claimed)
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.dispatch.Dispatch(batch)
	return count, nil
}
