package cases

import (
        stdcontext "context"

        casecontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/cases"
)

type Repository interface {
        Upsert(stdcontext.Context, casecontracts.Case) error
        Get(stdcontext.Context, tenantID, caseID string) (*casecontracts.Case, error)
}
