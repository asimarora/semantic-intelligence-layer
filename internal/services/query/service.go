package query

import (
        "context"
        "fmt"
        "strings"

        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        sessionstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

const (
        DefaultSessionLimit = 50
        MaxSessionLimit     = 200
)

type SessionService struct {
        Store sessionstore.Store
}

func NewSessionService(store sessionstore.Store) (*SessionService, error) {
        if store == nil {
                return nil, fmt.Errorf("session metadata store is required")
        }
        return &SessionService{Store: store}, nil
}

func (service *SessionService) SearchNetworkSessions(ctx context.Context, filter sessionstore.Query) ([]unifiedsessions.Event, error) {
        if service == nil || service.Store == nil {
                return nil, fmt.Errorf("session query service is not configured")
        }

        filter = normalizeQuery(filter)
        if err := validateQuery(filter); err != nil {
                return nil, err
        }

        return service.Store.Search(ctx, filter)
}

func normalizeQuery(filter sessionstore.Query) sessionstore.Query {
        filter.TenantID = strings.TrimSpace(filter.TenantID)
        filter.SubscriberID = strings.TrimSpace(filter.SubscriberID)
        filter.SessionID = strings.TrimSpace(filter.SessionID)
        filter.NASIPAddress = strings.TrimSpace(filter.NASIPAddress)
        filter.ClientIPAddress = strings.TrimSpace(filter.ClientIPAddress)

        if filter.From != nil {
                value := filter.From.UTC()
                filter.From = &value
        }
        if filter.To != nil {
                value := filter.To.UTC()
                filter.To = &value
        }

        switch {
        case filter.Limit <= 0:
                filter.Limit = DefaultSessionLimit
        case filter.Limit > MaxSessionLimit:
                filter.Limit = MaxSessionLimit
        }

        return filter
}

func validateQuery(filter sessionstore.Query) error {
        if filter.TenantID == "" {
                return fmt.Errorf("tenant_id is required")
        }
        if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
                return fmt.Errorf("from must be before or equal to to")
        }
        return nil
}
