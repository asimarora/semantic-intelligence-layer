package evidence

import (
        stdcontext "context"

        evidencecontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
)

type Repository interface {
        Append(stdcontext.Context, evidencecontracts.Record) error
        ListBySourceKey(stdcontext.Context, tenantID, source, sourceKey string, limit int) ([]evidencecontracts.Record, error)
}
