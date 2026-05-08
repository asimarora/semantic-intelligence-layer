package policy

import (
        "context"
        "strings"

        "github.com/asimarora/semantic-intelligence-layer/internal/agents"
)

type Rule struct {
        Name        string
        Description string
        Match       Match
        Decision    Decision
}

type Match struct {
        ToolNames            []string
        ToolPrefixes         []string
        SameTenantOnly       bool
        CrossTenant          bool
        ToolRequiresApproval bool
}

func (m Match) Matches(run *agents.Run, action Action) bool {
        if m.CrossTenant && !isCrossTenant(run, action) {
                return false
        }

        if m.SameTenantOnly && !isSameTenant(run, action) {
                return false
        }

        if m.ToolRequiresApproval && !action.Tool.RequiresApproval {
                return false
        }

        if len(m.ToolNames) == 0 && len(m.ToolPrefixes) == 0 {
                return true
        }

        for _, name := range m.ToolNames {
                if action.Tool.Name == name {
                        return true
                }
        }

        for _, prefix := range m.ToolPrefixes {
                if strings.HasPrefix(action.Tool.Name, prefix) {
                        return true
                }
        }

        return false
}

type StaticEvaluator struct {
        rules    []Rule
        fallback Decision
}

func NewStaticEvaluator(rules []Rule, fallback Decision) *StaticEvaluator {
        cloned := make([]Rule, len(rules))
        copy(cloned, rules)

        return &StaticEvaluator{
                rules:    cloned,
                fallback: fallback,
        }
}

func NewDefaultEvaluator() *StaticEvaluator {
        return NewStaticEvaluator(DefaultRules(), DefaultDecision())
}

func (e *StaticEvaluator) Evaluate(_ context.Context, run *agents.Run, action Action) (Decision, error) {
        for _, rule := range e.rules {
                if rule.Match.Matches(run, action) {
                        return rule.Decision, nil
                }
        }

        return e.fallback, nil
}

func DefaultRules() []Rule {
        return []Rule{
                {
                        Name:        "deny-cross-tenant",
                        Description: "Blocks tools from operating outside the run tenant boundary.",
                        Match: Match{
                                CrossTenant: true,
                        },
                        Decision: Decision{
                                State:  DecisionDeny,
                                Reason: "cross-tenant actions are not allowed",
                        },
                },
                {
                        Name:        "require-approval-for-state-change",
                        Description: "Requires human approval for state-changing or external tools.",
                        Match: Match{
                                ToolPrefixes: []string{"ticket.", "runbook.", "action.", "export."},
                        },
                        Decision: Decision{
                                State:  DecisionRequireApproval,
                                Reason: "state-changing or external tools require approval",
                        },
                },
                {
                        Name:        "require-approval-flagged-tool",
                        Description: "Honors tool definitions that explicitly require approval.",
                        Match: Match{
                                ToolRequiresApproval: true,
                        },
                        Decision: Decision{
                                State:  DecisionRequireApproval,
                                Reason: "tool is marked as requiring approval",
                        },
                },
                {
                        Name:        "allow-read-only-investigation",
                        Description: "Allows read-only investigation tools within the run tenant boundary.",
                        Match: Match{
                                ToolPrefixes:   []string{"search.", "retrieve.", "query.", "explain."},
                                SameTenantOnly: true,
                        },
                        Decision: Decision{
                                State:  DecisionAllow,
                                Reason: "read-only investigation tools are allowed",
                        },
                },
        }
}

func DefaultDecision() Decision {
        return Decision{
                State:  DecisionRequireApproval,
                Reason: "tool is not covered by a default allow policy",
        }
}

func isCrossTenant(run *agents.Run, action Action) bool {
        if run == nil || run.TenantID == "" || action.TargetTenantID == "" {
                return false
        }

        return run.TenantID != action.TargetTenantID
}

func isSameTenant(run *agents.Run, action Action) bool {
        if action.TargetTenantID == "" {
                return true
        }

        if run == nil || run.TenantID == "" {
                return false
        }

        return run.TenantID == action.TargetTenantID
}
