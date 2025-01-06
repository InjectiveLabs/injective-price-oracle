package pipeline

import (
	"context"
	"encoding/json"

	"go.uber.org/multierr"

	log "github.com/InjectiveLabs/suplog"
	"github.com/pkg/errors"
)

// Return types:
//
//	string
type HTTPTask struct {
	BaseTask    `mapstructure:",squash"`
	Method      string
	URL         string
	RequestData string `json:"requestData"`
	HeaderMap   string `json:"headerMap"`
}

var _ Task = (*HTTPTask)(nil)

func (t *HTTPTask) Type() TaskType {
	return TaskTypeHTTP
}

func (t *HTTPTask) Run(ctx context.Context, lggr log.Logger, vars Vars, inputs []Result) (result Result, runInfo RunInfo) {
	_, err := CheckInputs(inputs, -1, -1, 0)
	if err != nil {
		return Result{Error: errors.Wrap(err, "task inputs")}, runInfo
	}

	var (
		method      StringParam
		url         URLParam
		requestData MapParam
		headerMap   MapParam
	)
	err = multierr.Combine(
		errors.Wrap(ResolveParam(&method, From(NonemptyString(t.Method), "GET")), "method"),
		errors.Wrap(ResolveParam(&url, From(VarExpr(t.URL, vars), NonemptyString(t.URL))), "url"),
		errors.Wrap(ResolveParam(&requestData, From(VarExpr(t.RequestData, vars), JSONWithVarExprs(t.RequestData, vars, false), nil)), "requestData"),
		errors.Wrap(ResolveParam(&headerMap, From(VarExpr(t.HeaderMap, vars), JSONWithVarExprs(t.HeaderMap, vars, false), nil)), "headerMap"),
	)
	if err != nil {
		return Result{Error: err}, runInfo
	}

	requestDataJSON, err := json.Marshal(requestData)
	if err != nil {
		return Result{Error: err}, runInfo
	}

	headerMapJSON, err := json.Marshal(headerMap)
	if err != nil {
		return Result{Error: err}, runInfo
	}

	lggr.Debugln("HTTP task: sending request",
		"requestData", string(requestDataJSON),
		"headerMap", string(headerMapJSON),
		"url", url.String(),
		"method", method,
	)

	requestCtx, cancel := httpRequestCtx(ctx, t)
	defer cancel()

	responseBytes, statusCode, _, elapsed, err := makeHTTPRequest(requestCtx, lggr, method, url, requestData, headerMap)
	if err != nil {
		return Result{Error: err}, RunInfo{IsRetryable: isRetryableHTTPError(statusCode, err)}
	}

	_ = elapsed

	lggr.Debugln("HTTP task got response",
		"response", string(responseBytes),
		"url", url.String(),
		"dotID", t.DotID(),
	)

	// NOTE: We always stringify the response since this is required for all current jobs.
	// If a binary response is required we might consider adding an adapter
	// flag such as  "BinaryMode: true" which passes through raw binary as the
	// value instead.
	return Result{Value: string(responseBytes)}, runInfo
}
