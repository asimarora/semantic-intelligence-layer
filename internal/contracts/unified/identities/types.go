package identities

import "time"

type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Identity struct {
	ID          string       `json:"id"`
	TenantID    string       `json:"tenant_id"`
	DisplayName string       `json:"display_name,omitempty"`
	Identifiers []Identifier `json:"identifiers,omitempty"`
	UpdatedAt   time.Time    `json:"updated_at"`
}
