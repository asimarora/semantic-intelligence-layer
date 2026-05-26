package conversations

import (
	stdcontext "context"

	conversationcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/conversations"
)

type Repository interface {
	Upsert(ctx stdcontext.Context, value conversationcontracts.Conversation) error
	Get(ctx stdcontext.Context, tenantID string, conversationID string) (*conversationcontracts.Conversation, error)
}
