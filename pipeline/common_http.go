package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	log "github.com/InjectiveLabs/suplog"
)

func makeHTTPRequest(
	ctx context.Context,
	lggr log.Logger,
	method StringParam,
	url URLParam,
	requestData MapParam,
	headerMap MapParam,
) ([]byte, int, http.Header, time.Duration, error) {

	var bodyReader io.Reader
	if requestData != nil {
		bodyBytes, err := json.Marshal(requestData)
		if err != nil {
			return nil, 0, nil, 0, errors.Wrap(err, "failed to encode request body as JSON")
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	request, err := http.NewRequestWithContext(ctx, string(method), url.String(), bodyReader)
	if err != nil {
		return nil, 0, nil, 0, errors.Wrap(err, "failed to create http.Request")
	}
	request.Header.Set("Content-Type", "application/json")

	for key, value := range headerMap {
		if strings.ToLower(key) == lowerContentTypeKey {
			// skip Content-Type override attempts
			continue
		}

		request.Header.Set(key, value.(string))
	}

	httpRequest := HTTPRequest{
		Request: request,
		Logger: lggr.WithFields(log.Fields{
			"svc":    "pipeline",
			"action": "HTTPRequest",
		}),
	}

	start := time.Now()
	responseBytes, statusCode, headers, err := httpRequest.SendRequest()
	if ctx.Err() != nil {
		return nil, 0, nil, 0, errors.New("http request timed out or interrupted")
	}
	if err != nil {
		return nil, 0, nil, 0, errors.Wrapf(err, "error making http request")
	}
	elapsed := time.Since(start) // TODO: return elapsed from utils/http

	if statusCode >= 400 {
		maybeErr := bestEffortExtractError(responseBytes)
		return nil, statusCode, headers, 0, errors.Errorf("got error from %s: (status code %v) %s", url.String(), statusCode, maybeErr)
	}
	return responseBytes, statusCode, headers, elapsed, nil
}

var lowerContentTypeKey = strings.ToLower("Content-Type")

type PossibleErrorResponses struct {
	Error        string `json:"error"`
	ErrorMessage string `json:"errorMessage"`
}

func bestEffortExtractError(responseBytes []byte) string {
	var resp PossibleErrorResponses
	err := json.Unmarshal(responseBytes, &resp)
	if err != nil {
		return ""
	}
	if resp.Error != "" {
		return resp.Error
	} else if resp.ErrorMessage != "" {
		return resp.ErrorMessage
	}
	return string(responseBytes)
}

func httpRequestCtx(ctx context.Context, t Task) (requestCtx context.Context, cancel context.CancelFunc) {
	var defaultHTTPTimeout = 15 * time.Second

	if _, isSet := t.TaskTimeout(); !isSet && defaultHTTPTimeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, defaultHTTPTimeout)
	} else {
		requestCtx = ctx
		cancel = func() {}
	}
	return
}
