package interactions

import "time"

type Interaction struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ConversationID string    `json:"conversation_id"`
	Channel        string    `json:"channel,omitempty"`
	Body           string    `json:"body,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
}
