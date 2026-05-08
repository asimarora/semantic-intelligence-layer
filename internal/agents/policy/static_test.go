package policy

import (
        "context"
        "testing"

        "github.com/asimarora/semantic-intelligence-layer/internal/agents"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/tools"
)

func TestDefaultEvaluator(t *testing.T) {
        evaluator := NewDefaultEvaluator()
        run := &agents.Run{
                ID:       "run-1",
                TenantID: "tenant-a",
        }

        tests := []struct {
                name   string
                action Action
                want   DecisionState
        }{
                {
                        name: "allows read only search tools",
                        action: Action{
                                Tool: tools.Definition{Name: "search.sessions"},
                        },
                        want: DecisionAllow,
                },
                {
                        name: "requires approval for state changing tools",
                        action: Action{
                                Tool: tools.Definition{Name: "ticket.create"},
                        },
                        want: DecisionRequireApproval,
                },
                {
                        name: "requires approval for flagged tools",
                        action: Action{
                                Tool: tools.Definition{
                                        Name:             "query.hidden-evidence",
                                        RequiresApproval: true,
                                },
                        },
                        want: DecisionRequireApproval,
                },
                {
                        name: "denies cross tenant access",
                        action: Action{
                                Tool:           tools.Definition{Name: "search.sessions"},
                                TargetTenantID: "tenant-b",
                        },
                        want: DecisionDeny,
                },
                {
                        name: "requires approval for uncategorized tools",
                        action: Action{
                                Tool: tools.Definition{Name: "custom.lookup"},
                        },
                        want: DecisionRequireApproval,
                },
        }

        for _, tt := range tests {
                t.Run(tt.name, func(t *testing.T) {
                        decision, err := evaluator.Evaluate(context.Background(), run, tt.action)
                        if err != nil {
                                t.Fatalf("Evaluate() error = %v", err)
                        }

                        if decision.State != tt.want {
                                t.Fatalf("Evaluate() state = %q, want %q", decision.State, tt.want)
                        }
                })
        }
}
