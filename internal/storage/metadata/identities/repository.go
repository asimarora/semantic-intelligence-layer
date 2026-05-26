package identities

import (
	stdcontext "context"

	identitycontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/identities"
)

type Repository interface {
	Upsert(ctx stdcontext.Context, value identitycontracts.Identity) error
	Get(ctx stdcontext.Context, tenantID string, identityID string) (*identitycontracts.Identity, error)
	FindByIdentifier(ctx stdcontext.Context, tenantID string, identifierType string, identifierValue string) (*identitycontracts.Identity, error)
}
