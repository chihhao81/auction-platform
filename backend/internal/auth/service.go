// Package auth implements LINE Login, server-side sessions and the
// authentication / authorization middleware.
package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"auction-platform/backend/internal/apperr"
	"auction-platform/backend/internal/audit"
	"auction-platform/backend/internal/database/db"
	"auction-platform/backend/internal/httpx"
	"auction-platform/backend/internal/store"
	"auction-platform/backend/internal/terms"
)

const (
	RoleUser      = "USER"
	RoleAdmin     = "ADMIN"
	StatusActive  = "ACTIVE"
	StatusBanned  = "BANNED"
	maxNameLength = 100
)

var (
	ErrBanned          = apperr.Forbidden("USER_BANNED", "此帳號已被停權")
	ErrLineLoginFailed = apperr.Unauthorized("LINE_LOGIN_FAILED", "LINE 登入失敗，請重試")
)

// CurrentUser is the authenticated user attached to a request.
type CurrentUser struct {
	db.User
	TermsAccepted bool
}

func (u *CurrentUser) IsAdmin() bool { return u.Role == RoleAdmin }

type Service struct {
	store  *store.Store
	line   LineClient
	admins map[string]bool
	ttl    time.Duration
	now    func() time.Time
}

func NewService(st *store.Store, line LineClient, adminLineIDs map[string]bool, ttl time.Duration, now func() time.Time) *Service {
	return &Service{store: st, line: line, admins: adminLineIDs, ttl: ttl, now: now}
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      db.User
}

// Login completes LINE Login: it exchanges the code, creates or refreshes the
// user and opens a session. Banned users are rejected (and the attempt is
// audited) without creating a session.
func (s *Service) Login(ctx context.Context, code string) (LoginResult, error) {
	profile, err := s.line.Exchange(ctx, code)
	if err != nil {
		slog.WarnContext(ctx, "line exchange failed", "err", err)
		return LoginResult{}, ErrLineLoginFailed
	}
	role := RoleUser
	if s.admins[profile.UserID] {
		role = RoleAdmin
	}
	now := s.now().UTC()
	var res LoginResult
	banned := false
	err = s.store.InTx(ctx, func(q *db.Queries) error {
		row, err := q.UpsertUserFromLine(ctx, db.UpsertUserFromLineParams{
			LineUserID:  profile.UserID,
			DisplayName: sanitizeDisplayName(profile.DisplayName),
			AvatarUrl:   sanitizeAvatarURL(profile.PictureURL),
			Role:        role,
		})
		if err != nil {
			return fmt.Errorf("upsert user: %w", err)
		}
		user := db.User{
			ID: row.ID, LineUserID: row.LineUserID, DisplayName: row.DisplayName,
			AvatarUrl: row.AvatarUrl, Role: row.Role, Status: row.Status,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		target := user.ID.String()
		if row.Created {
			if err := audit.Record(ctx, q, now, audit.Entry{Actor: user.ID, Action: audit.ActionUserCreate, TargetType: audit.TargetUser, TargetID: target}); err != nil {
				return err
			}
		}
		if user.Status == StatusBanned {
			banned = true
			return audit.Record(ctx, q, now, audit.Entry{Actor: user.ID, Action: audit.ActionLoginRejectBanned, TargetType: audit.TargetUser, TargetID: target})
		}
		token := httpx.RandomToken()
		expires := now.Add(s.ttl)
		meta := httpx.MetaFromContext(ctx)
		if err := q.CreateSession(ctx, db.CreateSessionParams{
			TokenHash: hashToken(token), UserID: user.ID, Ip: meta.IP, UserAgent: meta.UserAgent,
			CreatedAt: now, ExpiresAt: expires,
		}); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		res = LoginResult{Token: token, ExpiresAt: expires, User: user}
		return audit.Record(ctx, q, now, audit.Entry{Actor: user.ID, Action: audit.ActionLogin, TargetType: audit.TargetUser, TargetID: target})
	})
	if err != nil {
		return LoginResult{}, err
	}
	if banned {
		return LoginResult{}, ErrBanned
	}
	return res, nil
}

// Authenticate resolves a session token. It returns (nil, nil) when the token
// is unknown, expired or revoked.
func (s *Service) Authenticate(ctx context.Context, token string) (*CurrentUser, error) {
	if token == "" {
		return nil, nil
	}
	row, err := s.store.GetActiveSessionUser(ctx, db.GetActiveSessionUserParams{
		TermsVersion: terms.CurrentVersion, TokenHash: hashToken(token), Now: s.now().UTC(),
	})
	if store.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup session: %w", err)
	}
	return &CurrentUser{User: row.User, TermsAccepted: row.TermsAccepted}, nil
}

// Logout revokes the session and records the logout.
func (s *Service) Logout(ctx context.Context, token string, user *CurrentUser) error {
	now := s.now().UTC()
	return s.store.InTx(ctx, func(q *db.Queries) error {
		if err := q.RevokeSession(ctx, db.RevokeSessionParams{Now: now, TokenHash: hashToken(token)}); err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}
		return audit.Record(ctx, q, now, audit.Entry{Actor: user.ID, Action: audit.ActionLogout, TargetType: audit.TargetUser, TargetID: user.ID.String()})
	})
}

// AcceptTerms records acceptance of version, which must be the current one.
func (s *Service) AcceptTerms(ctx context.Context, user *CurrentUser, version string) error {
	if version != terms.CurrentVersion {
		return apperr.Invalid("TERMS_VERSION_MISMATCH", "條款版本已更新，請重新閱讀")
	}
	now := s.now().UTC()
	return s.store.InTx(ctx, func(q *db.Queries) error {
		if err := q.AcceptTerms(ctx, db.AcceptTermsParams{UserID: user.ID, TermsVersion: version, AcceptedAt: now}); err != nil {
			return fmt.Errorf("accept terms: %w", err)
		}
		return audit.Record(ctx, q, now, audit.Entry{
			Actor: user.ID, Action: audit.ActionTermsAccept, TargetType: audit.TargetUser, TargetID: user.ID.String(),
			Metadata: map[string]any{"terms_version": version},
		})
	})
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func sanitizeDisplayName(name string) string {
	name = strings.TrimSpace(strings.ToValidUTF8(name, ""))
	if name == "" {
		return "LINE 使用者"
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		name = string([]rune(name)[:maxNameLength])
	}
	return name
}

// sanitizeAvatarURL only keeps https URLs so a profile picture can never be a
// javascript: or data: URL when rendered.
func sanitizeAvatarURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.String()
}

// IsBanned reports whether err is the banned-user error.
func IsBanned(err error) bool { return errors.Is(err, ErrBanned) }
