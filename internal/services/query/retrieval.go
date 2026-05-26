package query

import (
	"context"
	"fmt"
	"strings"

	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	retrievalstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
)

const (
	DefaultRetrievalLimit = 10
	MaxRetrievalLimit     = 50
)

type RetrievalService struct {
	Store retrievalstore.Store
}

func NewRetrievalService(store retrievalstore.Store) (*RetrievalService, error) {
	if store == nil {
		return nil, fmt.Errorf("retrieval metadata store is required")
	}
	return &RetrievalService{Store: store}, nil
}

func (service *RetrievalService) SearchEvidence(ctx context.Context, filter retrievalstore.Query) ([]retrievalcontracts.Hit, error) {
	if service == nil || service.Store == nil {
		return nil, fmt.Errorf("retrieval query service is not configured")
	}

	filter = normalizeRetrievalQuery(filter)
	if err := validateRetrievalQuery(filter); err != nil {
		return nil, err
	}

	return service.Store.Search(ctx, filter)
}

func normalizeRetrievalQuery(filter retrievalstore.Query) retrievalstore.Query {
	filter.TenantID = strings.TrimSpace(filter.TenantID)
	filter.QueryText = strings.TrimSpace(filter.QueryText)
	filter.SubscriberID = strings.TrimSpace(filter.SubscriberID)
	filter.SessionID = strings.TrimSpace(filter.SessionID)
	filter.RequestID = strings.TrimSpace(filter.RequestID)
	filter.Status = strings.TrimSpace(filter.Status)
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
		filter.Limit = DefaultRetrievalLimit
	case filter.Limit > MaxRetrievalLimit:
		filter.Limit = MaxRetrievalLimit
	}

	return filter
}

func validateRetrievalQuery(filter retrievalstore.Query) error {
	if filter.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if filter.QueryText == "" {
		return fmt.Errorf("query is required")
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return fmt.Errorf("from must be before or equal to to")
	}
	return nil
}
