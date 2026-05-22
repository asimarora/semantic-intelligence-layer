package query

import (
        "context"
        "fmt"
        "strings"

        unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
        accessstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
)

const (
        DefaultAccessLimit = 50
        MaxAccessLimit     = 200
)

type AccessService struct {
        Store accessstore.Store
}

func NewAccessService(store accessstore.Store) (*AccessService, error) {
        if store == nil {
                return nil, fmt.Errorf("access metadata store is required")
        }
        return &AccessService{Store: store}, nil
}

func (service *AccessService) SearchNetworkAccessEvents(ctx context.Context, filter accessstore.Query) ([]unifiedaccess.Event, error) {
        if service == nil || service.Store == nil {
                return nil, fmt.Errorf("access query service is not configured")
        }

        filter = normalizeAccessQuery(filter)
        if err := validateAccessQuery(filter); err != nil {
                return nil, err
        }
        return service.Store.Search(ctx, filter)
}

func normalizeAccessQuery(filter accessstore.Query) accessstore.Query {
        filter.TenantID = strings.TrimSpace(filter.TenantID)
        filter.SubscriberID = strings.TrimSpace(filter.SubscriberID)
        filter.SessionID = strings.TrimSpace(filter.SessionID)
        filter.Outcome = strings.TrimSpace(filter.Outcome)
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
                filter.Limit = DefaultAccessLimit
        case filter.Limit > MaxAccessLimit:
                filter.Limit = MaxAccessLimit
        }
        return filter
}

func validateAccessQuery(filter accessstore.Query) error {
        if filter.TenantID == "" {
                return fmt.Errorf("tenant_id is required")
        }
        if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
                return fmt.Errorf("from must be before or equal to to")
        }
        return nil
}
