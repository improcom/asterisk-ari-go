/*
 * Copyright (C) 2022 Quintex Software Solutions Pvt. Ltd. - All Rights Reserved.
 *
 * You may use, distribute and modify this code under the terms of the Apache
 * License Version 2.0. You should have received a copy of the license with this file.
 * If not, please write to : opensource@quintexsoftware.com
 *
 */

package asterisk_ari_go

import (
	"context"
	"fmt"
	"github.com/antihax/optional"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Linger please
var (
	_ context.Context
)

type WebsocketApiService service

/*
WebsocketApiService WebSocket connection for events.
 * @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 * @param app Applications to subscribe to.
 * @param optional nil or *EventsApiEventWebsocketOpts - Optional Parameters:
     * @param "SubscribeAll" (optional.Bool) -  Subscribe to all Asterisk events. If provided, the applications listed will be subscribed to all events, effectively disabling the application specific subscriptions. Default is &#39;false&#39;.

@return Message
*/

type WebsocketApiOpts struct {
	SubscribeAll optional.Bool
}

func (a *WebsocketApiService) WebsocketConnect(ctx context.Context, app []string, auth []string) (*websocket.Conn, *http.Response, error) {

	// Create the WebSocket URL
	u := url.URL{
		Scheme: a.client.cfg.Scheme,
		Host:   a.client.cfg.Host,
		Path:   a.client.cfg.BasePath + "/events",
	}

	a.client.logger.Debugf("Connecting to WebSocket. URL: %s", u.String())

	// Add query parameters
	query := u.Query()
	query.Add("app", strings.Join(app, ","))
	query.Add("api_key", strings.Join(auth, ","))
	u.RawQuery = query.Encode()

	// Create WebSocket connection
	headers := http.Header{}
	for key, value := range a.client.cfg.DefaultHeader {
		headers.Add(key, value)
	}
	if a.client.cfg.UserAgent != "" {
		headers.Set("User-Agent", a.client.cfg.UserAgent)
	}

	a.client.logger.Debugf("full URL: %s", u.String())
	a.client.logger.Debugf("headers: %v", headers)

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		var fullErrorMsg string
		if resp != nil && resp.Body != nil {
			body, _ := io.ReadAll(resp.Body)
			fullErrorMsg = fmt.Sprintf("failed to connect to websocket: %v. Resp: %v", err, string(body))
			return nil, resp, fmt.Errorf(fullErrorMsg)
		}
		return nil, resp, err
	}

	return conn, resp, nil
}
