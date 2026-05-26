package policy

import (
	"context"

	"github.com/asimarora/semantic-intelligence-layer/internal/agents"
	"github.com/asimarora/semantic-intelligence-layer/internal/agents/tools"
)

type DecisionState string

const (
	DecisionAllow           DecisionState = "allow"
	DecisionRequireApproval DecisionState = "require_approval"
	DecisionDeny            DecisionState = "deny"
)

type Action struct {
	RunID          string
	SessionID      string
	StepID         string
	Tool           tools.Definition
	TargetTenantID string
	Arguments      map[string]any
	Reason         string
}

type Decision struct {
	State  DecisionState
	Reason string
}

type Evaluator interface {
	Evaluate(ctx context.Context, run *agents.Run, action Action) (Decision, error)
}
