package pipeline

import (
	"context"

	log "github.com/InjectiveLabs/suplog"
)

type PanicTask struct {
	BaseTask `mapstructure:",squash"`
	Msg      string
}

var _ Task = (*PanicTask)(nil)

func (t *PanicTask) Type() TaskType {
	return TaskTypePanic
}

func (t *PanicTask) Run(_ context.Context, _ log.Logger, vars Vars, _ []Result) (result Result, runInfo RunInfo) {
	panic(t.Msg)
}
