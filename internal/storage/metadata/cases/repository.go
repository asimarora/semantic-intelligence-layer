package cases

import (
	stdcontext "context"

	casecontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/cases"
)

type Repository interface {
	Upsert(ctx stdcontext.Context, value casecontracts.Case) error
	Get(ctx stdcontext.Context, tenantID string, caseID string) (*casecontracts.Case, error)
}
