// Package network implements the capability-scoped network broker. Renderers
// send fetch requests over IPC; only the broker opens sockets.
package network

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gonetsurf/ipc"
	"gonetsurf/protocols/http1"
	"gonetsurf/storages/cookies"
	"gonetsurf/urls"
)

// Broker request actions.
const (
	ActionFetch   = "fetch"
	ActionPrivacy = "privacy"
	ActionReset   = "reset-tracking-session"
)

// Request is a broker request.
type Request struct {
	Action    string              `json:"action,omitempty"`
	URL       string              `json:"url"`
	Method    string              `json:"method,omitempty"`
	Partition string              `json:"partition,omitempty"`
	TopLevel  string              `json:"topLevel,omitempty"`
	Reload    bool                `json:"reload,omitempty"`
	Header    map[string][]string `json:"header,omitempty"`
}

// Response is a broker response.
type Response struct {
	Status    int                 `json:"status"`
	Header    map[string][]string `json:"header,omitempty"`
	Body      []byte              `json:"body,omitempty"`
	FromCache bool                `json:"fromCache,omitempty"`
	Privacy   *PrivacyStats       `json:"privacy,omitempty"`
}

// PrivacyStats describes the privacy state for one partition.
type PrivacyStats struct {
	Partition       string   `json:"partition"`
	TopLevel        string   `json:"topLevel,omitempty"`
	CookieCount     int      `json:"cookieCount"`
	ThirdPartyHosts []string `json:"thirdPartyHosts,omitempty"`
	CacheEntries    int      `json:"cacheEntries"`
	ResetCookies    int      `json:"resetCookies,omitempty"`
}

// Fetcher performs a validated request.
type Fetcher interface {
	Fetch(ctx context.Context, request Request) (Response, error)
}

// PrivacyProvider exposes privacy state and tracking-session resets.
type PrivacyProvider interface {
	Privacy(partition, topLevel string) PrivacyStats
	ResetTrackingSession(partition, topLevel string) PrivacyStats
}

// HTTPFetcher fetches HTTP(S) requests with the project HTTP client.
type HTTPFetcher struct {
	Client  *http1.Client
	Jar     *cookies.Jar
	Cache   *Cache
	Session *cookies.SessionManager
}

// Fetch implements Fetcher.
func (fetcher HTTPFetcher) Fetch(ctx context.Context, request Request) (Response, error) {
	target, err := urls.Parse(request.URL)
	if err != nil {
		return Response{}, err
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return Response{}, fmt.Errorf("network broker: unsupported scheme %q", target.Scheme)
	}
	method := request.Method
	if method == "" {
		method = "GET"
	}
	topLevel := request.TopLevel
	if topLevel == "" {
		topLevel = request.Partition
	}
	if fetcher.Session != nil {
		fetcher.Session.RecordRequest(topLevel, target.Hostname(), "")
	}
	if method == "GET" && !request.Reload && fetcher.Cache != nil {
		if cached, ok := fetcher.Cache.Get(request); ok {
			cached.FromCache = true
			return cached, nil
		}
	}
	header := http1.Header{}
	for name, values := range request.Header {
		for _, value := range values {
			header = append(header, http1.Field{Name: name, Value: value})
		}
	}
	partition := request.Partition
	if partition == "" {
		partition = "default"
	}
	now := time.Now()
	if fetcher.Jar != nil {
		cookieHeader := make([]string, 0)
		path := target.Pathname()
		if path == "" {
			path = "/"
		}
		for _, cookie := range fetcher.Jar.CookiesFor(partition, target.Hostname(), path, target.Scheme == "https", now) {
			cookieHeader = append(cookieHeader, cookie.Name+"="+cookie.Value)
		}
		if len(cookieHeader) > 0 {
			header = append(header, http1.Field{Name: "Cookie", Value: strings.Join(cookieHeader, "; ")})
		}
	}
	client := fetcher.Client
	if client == nil {
		client = &http1.Client{Transport: &http1.Transport{}}
	}
	response, err := client.Do(ctx, &http1.Request{Method: method, URL: target, Header: header})
	if err != nil {
		return Response{}, err
	}
	defer response.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return Response{}, err
	}
	headerMap := make(map[string][]string)
	for _, field := range response.Header {
		headerMap[field.Name] = append(headerMap[field.Name], field.Value)
	}
	if fetcher.Jar != nil {
		path := target.Pathname()
		if path == "" {
			path = "/"
		}
		for _, value := range response.Header.Values("Set-Cookie") {
			if cookie, ok := cookies.ParseSetCookie(target.Hostname(), path, value, now); ok {
				fetcher.Jar.Set(partition, cookie)
			}
		}
	}
	result := Response{Status: response.StatusCode, Header: headerMap, Body: body}
	if fetcher.Cache != nil {
		fetcher.Cache.Put(request, result)
	}
	return result, nil
}

// Server serves fetch requests over one IPC connection.
type Server struct {
	Conn    *ipc.Conn
	Token   ipc.Capability
	Fetcher Fetcher

	requestsMutex sync.Mutex
	requests      map[uint64]context.CancelFunc
	semaphore     chan struct{}
}

// Serve processes requests until the connection closes or ctx ends.
func (server *Server) Serve(ctx context.Context) error {
	fetcher := server.Fetcher
	if fetcher == nil {
		fetcher = HTTPFetcher{}
	}
	if server.requests == nil {
		server.requests = make(map[uint64]context.CancelFunc)
	}
	if server.semaphore == nil {
		server.semaphore = make(chan struct{}, 16)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		message, err := server.Conn.Receive()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if message.Kind == ipc.KindCancel {
			server.requestsMutex.Lock()
			cancel := server.requests[message.RequestID]
			server.requestsMutex.Unlock()
			if cancel != nil {
				cancel()
			}
			continue
		}
		if message.Kind != ipc.KindRequest {
			continue
		}
		if !tokensEqual(server.Token, message.Capability) {
			server.sendError(message.RequestID, "unauthorized capability")
			continue
		}
		var request Request
		if err := json.Unmarshal(message.Payload, &request); err != nil {
			server.sendError(message.RequestID, "invalid request")
			continue
		}
		requestContext, cancel := context.WithCancel(ctx)
		server.requestsMutex.Lock()
		server.requests[message.RequestID] = cancel
		server.requestsMutex.Unlock()
		go func(requestID uint64, request Request, requestContext context.Context, cancel context.CancelFunc) {
			defer func() {
				cancel()
				server.requestsMutex.Lock()
				delete(server.requests, requestID)
				server.requestsMutex.Unlock()
			}()
			server.semaphore <- struct{}{}
			defer func() { <-server.semaphore }()
			response, err := dispatchRequest(requestContext, fetcher, request)
			if err != nil {
				server.sendError(requestID, err.Error())
				return
			}
			payload, err := json.Marshal(response)
			if err != nil {
				server.sendError(requestID, err.Error())
				return
			}
			_ = server.Conn.Send(ipc.Message{Kind: ipc.KindResponse, RequestID: requestID, Capability: server.Token, Payload: payload})
		}(message.RequestID, request, requestContext, cancel)
	}
}

func (server *Server) sendError(requestID uint64, text string) {
	payload, _ := json.Marshal(text)
	_ = server.Conn.Send(ipc.Message{Kind: ipc.KindError, RequestID: requestID, Capability: server.Token, Payload: payload})
}

// Client issues fetch requests over one IPC connection.
type Client struct {
	Conn  *ipc.Conn
	Token ipc.Capability

	nextID       atomic.Uint64
	pendingMutex sync.Mutex
	pending      map[uint64]chan ipc.Message
	readOnce     sync.Once
	readErrMutex sync.Mutex
	readErr      error
}

// Fetch performs one broker fetch request.
func (client *Client) Fetch(ctx context.Context, request Request) (Response, error) {
	request.Action = ActionFetch
	return client.do(ctx, request)
}

// Privacy returns privacy metadata for a partition.
func (client *Client) Privacy(ctx context.Context, partition, topLevel string) (PrivacyStats, error) {
	response, err := client.do(ctx, Request{Action: ActionPrivacy, Partition: partition, TopLevel: topLevel})
	if err != nil {
		return PrivacyStats{}, err
	}
	if response.Privacy == nil {
		return PrivacyStats{}, fmt.Errorf("network broker: missing privacy metadata")
	}
	return *response.Privacy, nil
}

// ResetTrackingSession clears tracking-session state for a partition.
func (client *Client) ResetTrackingSession(ctx context.Context, partition, topLevel string) (PrivacyStats, error) {
	response, err := client.do(ctx, Request{Action: ActionReset, Partition: partition, TopLevel: topLevel})
	if err != nil {
		return PrivacyStats{}, err
	}
	if response.Privacy == nil {
		return PrivacyStats{}, fmt.Errorf("network broker: missing privacy metadata")
	}
	return *response.Privacy, nil
}

func (client *Client) do(ctx context.Context, request Request) (Response, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	requestID := client.nextID.Add(1)
	responses := make(chan ipc.Message, 1)
	client.pendingMutex.Lock()
	if client.pending == nil {
		client.pending = make(map[uint64]chan ipc.Message)
	}
	client.pending[requestID] = responses
	client.pendingMutex.Unlock()
	client.startReader()
	if err := client.Conn.Send(ipc.Message{Kind: ipc.KindRequest, RequestID: requestID, Capability: client.Token, Payload: payload}); err != nil {
		client.unregister(requestID)
		return Response{}, err
	}
	select {
	case <-ctx.Done():
		client.unregister(requestID)
		_ = client.Conn.Cancel(requestID)
		return Response{}, ctx.Err()
	case message, ok := <-responses:
		client.unregister(requestID)
		if !ok {
			return Response{}, client.readFailure()
		}
		switch message.Kind {
		case ipc.KindResponse:
			var response Response
			if err := json.Unmarshal(message.Payload, &response); err != nil {
				return Response{}, err
			}
			return response, nil
		case ipc.KindError:
			var text string
			_ = json.Unmarshal(message.Payload, &text)
			return Response{}, fmt.Errorf("network broker: %s", text)
		}
		return Response{}, fmt.Errorf("network broker: unexpected message")
	}
}

func (client *Client) startReader() {
	client.readOnce.Do(func() {
		go func() {
			for {
				message, err := client.Conn.Receive()
				if err != nil {
					client.readErrMutex.Lock()
					client.readErr = err
					client.readErrMutex.Unlock()
					client.pendingMutex.Lock()
					for requestID, channel := range client.pending {
						close(channel)
						delete(client.pending, requestID)
					}
					client.pendingMutex.Unlock()
					return
				}
				client.pendingMutex.Lock()
				channel := client.pending[message.RequestID]
				client.pendingMutex.Unlock()
				if channel != nil {
					select {
					case channel <- message:
					default:
					}
				}
			}
		}()
	})
}

func (client *Client) unregister(requestID uint64) {
	client.pendingMutex.Lock()
	delete(client.pending, requestID)
	client.pendingMutex.Unlock()
}

func (client *Client) readFailure() error {
	client.readErrMutex.Lock()
	defer client.readErrMutex.Unlock()
	if client.readErr != nil {
		return client.readErr
	}
	return fmt.Errorf("network broker: connection closed")
}

func dispatchRequest(ctx context.Context, fetcher Fetcher, request Request) (Response, error) {
	switch request.Action {
	case "", ActionFetch:
		return fetcher.Fetch(ctx, request)
	case ActionPrivacy, ActionReset:
		provider, ok := fetcher.(PrivacyProvider)
		if !ok {
			return Response{}, fmt.Errorf("network broker: privacy actions are unavailable")
		}
		var stats PrivacyStats
		if request.Action == ActionReset {
			stats = provider.ResetTrackingSession(request.Partition, request.TopLevel)
		} else {
			stats = provider.Privacy(request.Partition, request.TopLevel)
		}
		return Response{Status: 200, Privacy: &stats}, nil
	default:
		return Response{}, fmt.Errorf("network broker: unknown action %q", request.Action)
	}
}

// Privacy returns the current privacy state for a partition.
func (fetcher HTTPFetcher) Privacy(partition, topLevel string) PrivacyStats {
	if partition == "" {
		partition = "default"
	}
	if topLevel == "" {
		topLevel = partition
	}
	stats := PrivacyStats{Partition: partition, TopLevel: topLevel}
	if fetcher.Jar != nil {
		stats.CookieCount = len(fetcher.Jar.All(partition))
	}
	if fetcher.Session != nil {
		stats.ThirdPartyHosts = fetcher.Session.Graph().ClearableHosts(topLevel)
	}
	if fetcher.Cache != nil {
		stats.CacheEntries = fetcher.Cache.Count(partition)
	}
	return stats
}

// ResetTrackingSession clears tracking-session state for a partition.
func (fetcher HTTPFetcher) ResetTrackingSession(partition, topLevel string) PrivacyStats {
	if partition == "" {
		partition = "default"
	}
	if topLevel == "" {
		topLevel = partition
	}
	resetCookies := 0
	if fetcher.Session != nil {
		resetCookies = fetcher.Session.ResetTrackingSession(partition, topLevel, time.Now())
	}
	if fetcher.Cache != nil {
		fetcher.Cache.ClearPartition(partition)
	}
	stats := fetcher.Privacy(partition, topLevel)
	stats.ResetCookies = resetCookies
	return stats
}

func tokensEqual(first, second ipc.Capability) bool {
	return subtle.ConstantTimeCompare(first[:], second[:]) == 1
}
