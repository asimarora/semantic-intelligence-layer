package conversations

import "time"

type Conversation struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Channel       string    `json:"channel,omitempty"`
	CaseID        string    `json:"case_id,omitempty"`
	Subject       string    `json:"subject,omitempty"`
	LastMessageAt time.Time `json:"last_message_at"`
}
