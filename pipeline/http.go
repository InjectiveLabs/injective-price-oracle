package pipeline

import (
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/xlab/suplog"
)

// HTTPRequest holds the request and config struct for a http request
type HTTPRequest struct {
	Request *http.Request
	Logger  log.Logger
}

// SendRequest sends a HTTPRequest,
// returns a body, status code, and error.
func (h *HTTPRequest) SendRequest() (responseBody []byte, statusCode int, headers http.Header, err error) {
	var client *http.Client = &http.Client{}
	start := time.Now()

	r, err := client.Do(h.Request)
	if err != nil {
		h.Logger.WithError(err).Warningln("http adapter got error")
		return nil, 0, nil, err
	}

	defer r.Body.Close()

	statusCode = r.StatusCode
	elapsed := time.Since(start)
	h.Logger.Debugln(fmt.Sprintf("http adapter got %v in %s", statusCode, elapsed), "statusCode", statusCode, "timeElapsedSeconds", elapsed)

	source := http.MaxBytesReader(nil, r.Body, 10*1024*1024)
	bytes, err := io.ReadAll(source)
	if err != nil {
		h.Logger.Errorln("http adapter error reading body", "error", err)
		return nil, statusCode, nil, err
	}
	elapsed = time.Since(start)
	h.Logger.Debugln(fmt.Sprintf("http adapter finished after %s", elapsed), "statusCode", statusCode, "timeElapsedSeconds", elapsed)

	responseBody = bytes

	return responseBody, statusCode, r.Header, nil
}
