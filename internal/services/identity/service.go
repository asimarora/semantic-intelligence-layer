package identity

import (
        stdcontext "context"

        rawinteractions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/interactions"
        identitycontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/identities"
)

type ResolutionRequest struct {
        TenantID       string
        Channel        rawinteractions.Channel
        ConversationID string
        CustomerID     string
        Participants   []rawinteractions.Participant
        Metadata       map[string]string
}

type Resolution struct {
        Identity   identitycontracts.Identity
        Confidence float64
        MatchedBy  []string
}

type Resolver interface {
        Resolve(stdcontext.Context, ResolutionRequest) (*Resolution, error)
}
