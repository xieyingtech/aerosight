package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
)

// Preserve postgresstore's session format and cleanup while adding SCS's
// optional cancellable operations. Each lookup is bounded even for SSE.
type sessionStore struct {
	*postgresstore.PostgresStore
	queries *sqlcgen.Queries
	timeout time.Duration
}

var _ scs.CtxStore = (*sessionStore)(nil)

func (s *sessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	data, err := s.queries.FindHTTPSession(ctx, token)
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return data, err == nil, err
}
func (s *sessionStore) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	err := s.queries.CommitHTTPSession(ctx, sqlcgen.CommitHTTPSessionParams{Token: token, Data: data, Expiry: expiry})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
func (s *sessionStore) DeleteCtx(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	err := s.queries.DeleteHTTPSession(ctx, token)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
