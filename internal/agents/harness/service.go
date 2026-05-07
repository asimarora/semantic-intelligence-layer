package harness

import (
        "context"

        "github.com/asimarora/semantic-intelligence-layer/internal/agents"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/memory"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/policy"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/tools"
)

type Coordinator interface {
        StartRun(ctx context.Context, request agents.RunRequest) (*agents.Run, error)
        ContinueRun(ctx context.Context, runID string) (*agents.Run, error)
}

type Planner interface {
        BuildSteps(ctx context.Context, run *agents.Run, history []agents.Message) ([]agents.Step, error)
}

type Dependencies struct {
        Memory memory.Store
        Policy policy.Evaluator
        Tools  tools.Registry
}
