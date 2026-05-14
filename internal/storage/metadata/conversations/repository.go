package conversations

import (
        stdcontext "context"

        conversationcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/conversations"
)

type Repository interface {
        Upsert(stdcontext.Context, conversationcontracts.Conversation) error
        Get(stdcontext.Context, tenantID, conversationID string) (*conversationcontracts.Conversation, error)
}
