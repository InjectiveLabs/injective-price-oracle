package pipeline

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	log "github.com/xlab/suplog"
	null "gopkg.in/guregu/null.v4"

	cnull "github.com/InjectiveLabs/injective-price-oracle/pipeline/null"
)

type (
	Task interface {
		Type() TaskType
		ID() int
		DotID() string
		Run(ctx context.Context, lggr log.Logger, vars Vars, inputs []Result) (Result, RunInfo)
		Base() *BaseTask
		Outputs() []Task
		Inputs() []TaskDependency
		OutputIndex() int32
		TaskTimeout() (time.Duration, bool)
		TaskRetries() uint32
		TaskMinBackoff() time.Duration
		TaskMaxBackoff() time.Duration
	}
)

// Wraps the input Task for the given dependent task along with a bool variable PropagateResult,
// which Indicates whether result of InputTask should be propagated to its dependent task.
// If the edge between these tasks was an implicit edge, then results are not propagated. This is because
// some tasks cannot handle an input from an edge which wasn't specified in the spec.
type TaskDependency struct {
	PropagateResult bool
	InputTask       Task
}

var (
	ErrWrongInputCardinality = errors.New("wrong number of task inputs")
	ErrBadInput              = errors.New("bad input for task")
	ErrInputTaskErrored      = errors.New("input task errored")
	ErrParameterEmpty        = errors.New("parameter is empty")
	ErrTooManyErrors         = errors.New("too many errors")
	ErrTimeout               = errors.New("timeout")
	ErrTaskRunFailed         = errors.New("task run failed")
	ErrCancelled             = errors.New("task run cancelled (fail early)")
)

const (
	InputTaskKey = "input"
)

// RunInfo contains additional information about the finished TaskRun
type RunInfo struct {
	IsRetryable bool
	IsPending   bool
}

// retryableMeta should be returned if the error is non-deterministic; i.e. a
// repeated attempt sometime later _might_ succeed where the current attempt
// failed
func retryableRunInfo() RunInfo {
	return RunInfo{IsRetryable: true}
}

func pendingRunInfo() RunInfo {
	return RunInfo{IsPending: true}
}

func isRetryableHTTPError(statusCode int, err error) bool {
	if statusCode >= 400 && statusCode < 500 {
		// Client errors are not likely to succeed by resubmitting the exact same information again
		return false
	} else if statusCode >= 500 {
		// Remote errors _might_ work on a retry
		return true
	}
	return err != nil
}

// Result is the result of a TaskRun
type Result struct {
	Value interface{}
	Error error
}

// OutputDB dumps a single result output for a pipeline_run or pipeline_task_run
func (result Result) OutputDB() JSONSerializable {
	return JSONSerializable{
		Val:   result.Value,
		Valid: !(result.Value == nil || (reflect.ValueOf(result.Value).Kind() == reflect.Ptr && reflect.ValueOf(result.Value).IsNil())),
	}
}

// ErrorDB dumps a single result error for a pipeline_task_run
func (result Result) ErrorDB() null.String {
	var errString null.String
	if result.Error != nil {
		errString = null.StringFrom(result.Error.Error())
	}
	return errString
}

// FinalResult is the result of a Run
type FinalResult struct {
	Values      []interface{}
	AllErrors   []error
	FatalErrors []error
}

// HasFatalErrors returns true if the final result has any errors
func (result FinalResult) HasFatalErrors() bool {
	for _, err := range result.FatalErrors {
		if err != nil {
			return true
		}
	}
	return false
}

// HasErrors returns true if the final result has any errors
func (result FinalResult) HasErrors() bool {
	for _, err := range result.AllErrors {
		if err != nil {
			return true
		}
	}
	return false
}

// SingularResult returns a single result if the FinalResult only has one set of outputs/errors
func (result FinalResult) SingularResult() (Result, error) {
	if len(result.FatalErrors) != 1 || len(result.Values) != 1 {
		return Result{}, errors.Errorf("cannot cast FinalResult to singular result; it does not have exactly 1 error and exactly 1 output: %#v", result)
	}
	return Result{Error: result.FatalErrors[0], Value: result.Values[0]}, nil
}

// TaskRunResult describes the result of a task run, suitable for database
// update or insert.
// ID might be zero if the TaskRun has not been inserted yet
// TaskSpecID will always be non-zero
type TaskRunResult struct {
	ID         uuid.UUID
	Task       Task
	TaskRun    TaskRun
	Result     Result
	Attempts   uint
	CreatedAt  time.Time
	FinishedAt null.Time
	// runInfo is never persisted
	runInfo RunInfo
}

func (result *TaskRunResult) IsPending() bool {
	return !result.FinishedAt.Valid && result.Result == Result{}
}

func (result *TaskRunResult) IsTerminal() bool {
	return len(result.Task.Outputs()) == 0
}

// TaskRunResults represents a collection of results for all task runs for one pipeline run
type TaskRunResults []TaskRunResult

// FinalResult pulls the FinalResult for the pipeline_run from the task runs
// It needs to respect the output index of each task
func (trrs TaskRunResults) FinalResult(l log.Logger) FinalResult {
	var found bool
	var fr FinalResult
	sort.Slice(trrs, func(i, j int) bool {
		return trrs[i].Task.OutputIndex() < trrs[j].Task.OutputIndex()
	})
	for _, trr := range trrs {
		fr.AllErrors = append(fr.AllErrors, trr.Result.Error)
		if trr.IsTerminal() {
			fr.Values = append(fr.Values, trr.Result.Value)
			fr.FatalErrors = append(fr.FatalErrors, trr.Result.Error)
			found = true
		}
	}

	if !found {
		l.Panicln("Expected at least one task to be final", "tasks", trrs)
	}
	return fr
}

type JSONSerializable struct {
	Val   interface{}
	Valid bool
}

// UnmarshalJSON implements custom unmarshaling logic
func (js *JSONSerializable) UnmarshalJSON(bs []byte) error {
	if js == nil {
		*js = JSONSerializable{}
	}
	str := string(bs)
	if str == "" || str == "null" {
		js.Valid = false
		return nil
	}

	err := json.Unmarshal(bs, &js.Val)
	js.Valid = err == nil
	return err
}

// MarshalJSON implements custom marshaling logic
func (js JSONSerializable) MarshalJSON() ([]byte, error) {
	if !js.Valid {
		return []byte("null"), nil
	}
	switch x := js.Val.(type) {
	case []byte:
		// Don't need to HEX encode if it is a valid JSON string
		if json.Valid(x) {
			return json.Marshal(string(x))
		}

		// Don't need to HEX encode if it is already HEX encoded value
		if isHexBytes(x) {
			return json.Marshal(string(x))
		}

		return json.Marshal(hex.EncodeToString(x))
	default:
		return json.Marshal(js.Val)
	}
}

// isHexBytes returns true if the given bytes array is basically HEX encoded value.
func isHexBytes(arr []byte) bool {
	_, err := hex.DecodeString(strings.TrimPrefix(string(arr), "0x"))
	return err == nil
}

func (js *JSONSerializable) Scan(value interface{}) error {
	if value == nil {
		*js = JSONSerializable{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.Errorf("JSONSerializable#Scan received a value of type %T", value)
	}
	if js == nil {
		*js = JSONSerializable{}
	}
	return js.UnmarshalJSON(bytes)
}

func (js JSONSerializable) Value() (driver.Value, error) {
	if !js.Valid {
		return nil, nil
	}
	return js.MarshalJSON()
}

func (js *JSONSerializable) Empty() bool {
	return js == nil || !js.Valid
}

type TaskType string

func (t TaskType) String() string {
	return string(t)
}

const (
	TaskTypeHTTP            TaskType = "http"
	TaskTypeMean            TaskType = "mean"
	TaskTypeMedian          TaskType = "median"
	TaskTypeMode            TaskType = "mode"
	TaskTypeSum             TaskType = "sum"
	TaskTypeMultiply        TaskType = "multiply"
	TaskTypeDivide          TaskType = "divide"
	TaskTypeJSONParse       TaskType = "jsonparse"
	TaskTypeAny             TaskType = "any"
	TaskTypeETHABIEncode    TaskType = "ethabiencode"
	TaskTypeETHABIEncode2   TaskType = "ethabiencode2"
	TaskTypeETHABIDecode    TaskType = "ethabidecode"
	TaskTypeETHABIDecodeLog TaskType = "ethabidecodelog"
	TaskTypeMerge           TaskType = "merge"
	TaskTypeLowercase       TaskType = "lowercase"
	TaskTypeUppercase       TaskType = "uppercase"

	// Testing only.
	TaskTypePanic TaskType = "panic"
	TaskTypeMemo  TaskType = "memo"
	TaskTypeFail  TaskType = "fail"
)

var (
	stringType     = reflect.TypeOf("")
	bytesType      = reflect.TypeOf([]byte(nil))
	bytes20Type    = reflect.TypeOf([20]byte{})
	int32Type      = reflect.TypeOf(int32(0))
	nullUint32Type = reflect.TypeOf(cnull.Uint32{})
)

func UnmarshalTaskFromMap(taskType TaskType, taskMap interface{}, ID int, dotID string) (_ Task, err error) {
	defer func(err *error) {
		if *err != nil {
			*err = errors.Wrap(*err, "UnmarshalTaskFromMap")
		}
	}(&err)

	switch taskMap.(type) {
	default:
		return nil, errors.Errorf("UnmarshalTaskFromMap only accepts a map[string]interface{} or a map[string]string. Got %v (%#v) of type %T", taskMap, taskMap, taskMap)
	case map[string]interface{}, map[string]string:
	}

	taskType = TaskType(strings.ToLower(string(taskType)))

	var task Task
	switch taskType {
	case TaskTypePanic:
		task = &PanicTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeHTTP:
		task = &HTTPTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeMean:
		task = &MeanTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeMedian:
		task = &MedianTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeMode:
		task = &ModeTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeSum:
		task = &SumTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeAny:
		task = &AnyTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeJSONParse:
		task = &JSONParseTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeMemo:
		task = &MemoTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeMultiply:
		task = &MultiplyTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeDivide:
		task = &DivideTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeETHABIEncode:
		task = &ETHABIEncodeTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeETHABIEncode2:
		task = &ETHABIEncodeTask2{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeETHABIDecode:
		task = &ETHABIDecodeTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeETHABIDecodeLog:
		task = &ETHABIDecodeLogTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeFail:
		task = &FailTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeMerge:
		task = &MergeTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeLowercase:
		task = &LowercaseTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	case TaskTypeUppercase:
		task = &UppercaseTask{BaseTask: BaseTask{id: ID, dotID: dotID}}
	default:
		return nil, errors.Errorf(`unknown task type: "%v"`, taskType)
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           task,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
				if from != stringType {
					return data, nil
				}
				switch to {
				case nullUint32Type:
					i, err2 := strconv.ParseUint(data.(string), 10, 32)
					return cnull.Uint32From(uint32(i)), err2
				}
				return data, nil
			},
		),
	})
	if err != nil {
		return nil, err
	}

	err = decoder.Decode(taskMap)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func CheckInputs(inputs []Result, minLen, maxLen, maxErrors int) ([]interface{}, error) {
	if minLen >= 0 && len(inputs) < minLen {
		return nil, errors.Wrapf(ErrWrongInputCardinality, "min: %v max: %v (got %v)", minLen, maxLen, len(inputs))
	} else if maxLen >= 0 && len(inputs) > maxLen {
		return nil, errors.Wrapf(ErrWrongInputCardinality, "min: %v max: %v (got %v)", minLen, maxLen, len(inputs))
	}
	var vals []interface{}
	var errs int
	for _, input := range inputs {
		if input.Error != nil {
			errs++
			continue
		}
		vals = append(vals, input.Value)
	}
	if maxErrors >= 0 && errs > maxErrors {
		return nil, ErrTooManyErrors
	}
	return vals, nil
}
