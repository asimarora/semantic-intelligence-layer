package agents

import "time"

type RunStatus string

const (
        RunStatusPending          RunStatus = "pending"
        RunStatusRunning          RunStatus = "running"
        RunStatusAwaitingApproval RunStatus = "awaiting_approval"
        RunStatusCompleted        RunStatus = "completed"
        RunStatusFailed           RunStatus = "failed"
)

type StepStatus string

const (
        StepStatusPending   StepStatus = "pending"
        StepStatusRunning   StepStatus = "running"
        StepStatusBlocked   StepStatus = "blocked"
        StepStatusCompleted StepStatus = "completed"
        StepStatusFailed    StepStatus = "failed"
)

type MessageRole string

const (
        MessageRoleSystem    MessageRole = "system"
        MessageRoleAnalyst   MessageRole = "analyst"
        MessageRoleAssistant MessageRole = "assistant"
        MessageRoleTool      MessageRole = "tool"
)

type Session struct {
        ID       string
        TenantID string
        CaseID   string
        Goal     string
        OpenedAt time.Time
        UpdatedAt time.Time
}

type RunRequest struct {
        TenantID  string
        CaseID    string
        SessionID string
        Goal      string
        Inputs    map[string]string
        MaxSteps  int
}

type Run struct {
        ID          string
        TenantID    string
        CaseID      string
        SessionID   string
        Goal        string
        Status      RunStatus
        StartedAt   time.Time
        UpdatedAt   time.Time
        CompletedAt *time.Time
}

type Message struct {
        Role      MessageRole
        Content   string
        CreatedAt time.Time
}

type EvidenceRef struct {
        Kind    string
        ID      string
        Source  string
        Summary string
        URI     string
        Score   float64
}

type Step struct {
        ID          string
        Kind        string
        Summary     string
        ToolName    string
        Status      StepStatus
        Evidence    []EvidenceRef
        StartedAt   time.Time
        CompletedAt *time.Time
}
