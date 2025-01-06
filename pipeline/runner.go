package pipeline

import (
	"context"
	"fmt"
	"sort"
	"time"

	log "github.com/InjectiveLabs/suplog"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	null "gopkg.in/guregu/null.v4"
)

type Runner interface {
	// ExecuteRun executes a new run in-memory according to a spec and returns the results.
	ExecuteRun(ctx context.Context, spec Spec, vars Vars, l log.Logger) (run Run, trrs TaskRunResults, err error)
}

type runner struct {
	lggr log.Logger
}

func NewRunner(lggr log.Logger) *runner {
	r := &runner{
		lggr: lggr.WithField("svc", "PipelineRunner"),
	}

	return r
}

type memoryTaskRun struct {
	task     Task
	inputs   []Result // sorted by input index
	vars     Vars
	attempts uint
}

// When a task panics, we catch the panic and wrap it in an error for reporting to the scheduler.
type ErrRunPanicked struct {
	v interface{}
}

func (err ErrRunPanicked) Error() string {
	return fmt.Sprintf("goroutine panicked when executing run: %v", err.v)
}

func NewRun(spec Spec, vars Vars) Run {
	return Run{
		State:          RunStatusRunning,
		PipelineSpec:   spec,
		PipelineSpecID: spec.ID,
		Inputs:         JSONSerializable{Val: vars.vars, Valid: true},
		Outputs:        JSONSerializable{Val: nil, Valid: false},
		CreatedAt:      time.Now(),
	}
}

func (r *runner) ExecuteRun(
	ctx context.Context,
	spec Spec,
	vars Vars,
	l log.Logger,
) (Run, TaskRunResults, error) {
	run := NewRun(spec, vars)

	pipeline, err := r.initializePipeline(&run)

	if err != nil {
		return run, nil, err
	}

	taskRunResults, err := r.run(ctx, pipeline, &run, vars, l)
	if err != nil {
		return run, nil, err
	}

	if run.Pending {
		return run, nil, errors.Wrapf(err, "unexpected async run for spec ID %v, tried executing via ExecuteAndInsertFinishedRun", spec.ID)
	}

	return run, taskRunResults, nil
}

func (r *runner) initializePipeline(run *Run) (*Pipeline, error) {
	pipeline, err := Parse(run.PipelineSpec.DotDagSource)
	if err != nil {
		return nil, err
	}

	// initialize certain task params
	for _, task := range pipeline.Tasks {
		task.Base().uuid = uuid.NewV4()
	}

	// retain old UUID values
	for _, taskRun := range run.PipelineTaskRuns {
		task := pipeline.ByDotID(taskRun.DotID)
		task.Base().uuid = taskRun.ID
	}

	return pipeline, nil
}

func (r *runner) run(
	ctx context.Context,
	pipeline *Pipeline,
	run *Run,
	vars Vars,
	l log.Logger,
) (TaskRunResults, error) {
	l.Debugln("Initiating tasks for pipeline run of spec", "job ID", run.PipelineSpec.JobID, "job name", run.PipelineSpec.JobName)

	scheduler := newScheduler(pipeline, run, vars, l)
	go scheduler.Run()

	// This is "just in case" for cleaning up any stray reports.
	// Normally the scheduler loop doesn't stop until all in progress runs report back
	reportCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	for taskRun := range scheduler.taskCh {
		taskRun := taskRun
		// execute
		go WrapRecoverHandle(l, func() {
			result := r.executeTaskRun(ctx, run.PipelineSpec, taskRun, l)

			scheduler.report(reportCtx, result)
		}, func(err interface{}) {
			t := time.Now()
			scheduler.report(reportCtx, TaskRunResult{
				ID:         uuid.NewV4(),
				Task:       taskRun.task,
				Result:     Result{Error: ErrRunPanicked{err}},
				FinishedAt: null.TimeFrom(t),
				CreatedAt:  t, // TODO: more accurate start time
			})
		})
	}

	// if the run is suspended, awaiting resumption
	run.Pending = scheduler.pending
	run.FailEarly = scheduler.exiting
	run.State = RunStatusSuspended

	if !scheduler.pending {
		run.FinishedAt = null.TimeFrom(time.Now())

		// NOTE: runTime can be very long now because it'll include suspend
		runTime := run.FinishedAt.Time.Sub(run.CreatedAt)
		l.Debugln("Finished all tasks for pipeline run", "specID", run.PipelineSpecID, "runTime", runTime)
	}

	// Update run results
	run.PipelineTaskRuns = nil
	for _, result := range scheduler.results {
		output := result.Result.OutputDB()
		run.PipelineTaskRuns = append(run.PipelineTaskRuns, TaskRun{
			ID:            result.ID,
			PipelineRunID: run.ID,
			Type:          result.Task.Type(),
			Index:         result.Task.OutputIndex(),
			Output:        output,
			Error:         result.Result.ErrorDB(),
			DotID:         result.Task.DotID(),
			CreatedAt:     result.CreatedAt,
			FinishedAt:    result.FinishedAt,
			task:          result.Task,
		})

		sort.Slice(run.PipelineTaskRuns, func(i, j int) bool {
			return run.PipelineTaskRuns[i].task.OutputIndex() < run.PipelineTaskRuns[j].task.OutputIndex()
		})
	}

	// Update run errors/outputs
	if run.FinishedAt.Valid {
		var errors []null.String
		var fatalErrors []null.String
		var outputs []interface{}
		for _, result := range run.PipelineTaskRuns {
			if result.Error.Valid {
				errors = append(errors, result.Error)
			}
			// skip non-terminal results
			if len(result.task.Outputs()) != 0 {
				continue
			}
			fatalErrors = append(fatalErrors, result.Error)
			outputs = append(outputs, result.Output.Val)
		}
		run.AllErrors = errors
		run.FatalErrors = fatalErrors
		run.Outputs = JSONSerializable{Val: outputs, Valid: true}

		if run.HasFatalErrors() {
			run.State = RunStatusErrored
		} else {
			run.State = RunStatusCompleted
		}
	}

	// TODO: drop this once we stop using TaskRunResults
	var taskRunResults TaskRunResults
	for _, result := range scheduler.results {
		taskRunResults = append(taskRunResults, result)
	}

	return taskRunResults, nil
}

func (r *runner) executeTaskRun(ctx context.Context, spec Spec, taskRun *memoryTaskRun, l log.Logger) TaskRunResult {
	start := time.Now()
	l = l.WithFields(log.Fields{
		"taskName": taskRun.task.DotID(),
		"taskType": taskRun.task.Type(),
		"attempt":  taskRun.attempts,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if taskTimeout, isSet := taskRun.task.TaskTimeout(); isSet && taskTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, taskTimeout)
		defer cancel()
	}

	result, runInfo := taskRun.task.Run(ctx, l, taskRun.vars, taskRun.inputs)
	loggerFields := log.Fields{"runInfo": runInfo,
		"resultValue": result.Value,
		"resultError": result.Error,
		"resultType":  fmt.Sprintf("%T", result.Value),
	}
	switch v := result.Value.(type) {
	case []byte:
		loggerFields["resultString"] = fmt.Sprintf("%q", v)
		loggerFields["resultHex"] = fmt.Sprintf("%x", v)
	}

	l.WithFields(loggerFields).Debugln("Pipeline task completed")

	now := time.Now()

	var finishedAt null.Time
	if !runInfo.IsPending {
		finishedAt = null.TimeFrom(now)
	}
	return TaskRunResult{
		ID:         taskRun.task.Base().uuid,
		Task:       taskRun.task,
		Result:     result,
		CreatedAt:  start,
		FinishedAt: finishedAt,
		runInfo:    runInfo,
	}
}

func WrapRecoverHandle(lggr log.Logger, fn func(), onPanic func(interface{})) {
	defer func() {
		if err := recover(); err != nil {
			lggr.WithError(err.(error)).Errorln("recovered after panic")

			if onPanic != nil {
				onPanic(err)
			}
		}
	}()
	fn()
}
