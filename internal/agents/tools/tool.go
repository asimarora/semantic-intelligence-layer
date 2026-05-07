package tools

import (
        "context"

        "github.com/asimarora/semantic-intelligence-layer/internal/agents"
)

type Definition struct {
        Name             string
        Description      string
        RequiresApproval bool
}

type Invocation struct {
        RunID     string
        StepID    string
        Name      string
        Arguments map[string]any
}

type Result struct {
        Output   string
        Evidence []agents.EvidenceRef
}

type Tool interface {
        Definition() Definition
        Execute(ctx context.Context, call Invocation) (*Result, error)
}

type Registry interface {
        Resolve(name string) (Tool, bool)
        List() []Definition
}
