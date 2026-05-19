package cases

import (
        stdcontext "context"

        casecontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/cases"
)

type OpenRequest struct {
        TenantID           string
        CustomerIdentityID string
        PrimaryChannel     string
        Subject            string
        Summary            string
        Priority           casecontracts.Priority
        Attributes         map[string]string
}

type Service interface {
        Open(stdcontext.Context, OpenRequest) (*casecontracts.Case, error)
        Update(stdcontext.Context, casecontracts.Case) error
        Get(ctx stdcontext.Context, tenantID, caseID string) (*casecontracts.Case, error)
}
