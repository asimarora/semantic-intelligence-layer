package identities

import (
        stdcontext "context"

        identitycontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/identities"
)

type Repository interface {
        Upsert(stdcontext.Context, identitycontracts.Identity) error
        Get(stdcontext.Context, tenantID, identityID string) (*identitycontracts.Identity, error)
        FindByIdentifier(stdcontext.Context, tenantID, identifierType, identifierValue string) (*identitycontracts.Identity, error)
}
