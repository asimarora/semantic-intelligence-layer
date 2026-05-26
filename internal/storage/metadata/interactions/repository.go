package interactions

import (
	stdcontext "context"

	interactioncontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/interactions"
)

type Repository interface {
	Append(ctx stdcontext.Context, value interactioncontracts.Interaction) error
	ListByConversation(ctx stdcontext.Context, tenantID string, conversationID string, limit int) ([]interactioncontracts.Interaction, error)
}
