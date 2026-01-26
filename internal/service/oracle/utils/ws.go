package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"time"

	log "github.com/InjectiveLabs/suplog"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

func ConnectWebSocket(ctx context.Context, websocketUrl, urlHeader string, maxRetries int) (conn *websocket.Conn, err error) {
	u, err := url.Parse(websocketUrl)
	if err != nil {
		return &websocket.Conn{}, errors.Wrapf(err, "can not parse WS url %s: %v", websocketUrl, err)
	}

	header := http.Header{}
	if urlHeader != "" {
		header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(urlHeader)))
	}

	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true
	retries := 0
	for {
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, u.String(), header)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		} else if err != nil {
			log.Infof("Failed to connect to WebSocket server: %v", err)
			retries++
			if retries > maxRetries {
				log.Infof("Reached maximum retries (%d), exiting...", maxRetries)
				return nil, errors.New("reached maximum retries")
			}
			log.Infof("Retrying connect %sth in 5s...", fmt.Sprint(retries))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.NewTimer(5 * time.Second).C:
			}
		} else {
			log.Infof("Connected to WebSocket server")
			return
		}
	}
}
