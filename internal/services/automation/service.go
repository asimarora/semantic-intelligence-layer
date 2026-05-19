package automation

import (
        stdcontext "context"
        "time"
)

type Action struct {
        Name             string
        Description      string
        RequiresApproval bool
        Arguments        map[string]string
}

type Run struct {
        ID        string
        TenantID  string
        CaseID    string
        Channel   string
        Actions   []Action
        StartedAt time.Time
}

type Service interface {
        Start(stdcontext.Context, Run) error
}
