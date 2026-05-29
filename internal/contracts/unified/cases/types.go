package cases

import "time"

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

type Case struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenant_id"`
	CustomerIdentityID string            `json:"customer_identity_id,omitempty"`
	PrimaryChannel     string            `json:"primary_channel,omitempty"`
	Subject            string            `json:"subject,omitempty"`
	Summary            string            `json:"summary,omitempty"`
	Priority           Priority          `json:"priority,omitempty"`
	Attributes         map[string]string `json:"attributes,omitempty"`
	OpenedAt           time.Time         `json:"opened_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}
