package evidence

import (
	stdcontext "context"

	evidencecontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
)

type Repository interface {
	Append(ctx stdcontext.Context, value evidencecontracts.Record) error
	ListBySourceKey(ctx stdcontext.Context, tenantID string, source string, sourceKey string, limit int) ([]evidencecontracts.Record, error)
}
