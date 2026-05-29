package interactions

import "time"

type Channel string

type Participant struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name,omitempty"`
	Role        string            `json:"role,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

type Interaction struct {
	ID             string        `json:"id"`
	TenantID       string        `json:"tenant_id"`
	Channel        Channel       `json:"channel"`
	ConversationID string        `json:"conversation_id"`
	Participants   []Participant `json:"participants,omitempty"`
	OccurredAt     time.Time     `json:"occurred_at"`
}
