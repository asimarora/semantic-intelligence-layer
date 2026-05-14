package interactions

import (
        stdcontext "context"

        interactioncontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/interactions"
)

type Repository interface {
        Append(stdcontext.Context, interactioncontracts.Interaction) error
        ListByConversation(stdcontext.Context, tenantID, conversationID string, limit int) ([]interactioncontracts.Interaction, error)
}
