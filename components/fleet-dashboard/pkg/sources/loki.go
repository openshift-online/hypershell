package sources

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/websocket"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// LokiClient is the application-owned port the e2e-run plane reads through
// (e2e-run-log-visibility.spec E2E-LOG-03). It is expressed in log-access terms
// (history query and live tail), never Loki HTTP shapes, so the source and the
// stream handler depend only on this interface and the Loki adapter is the only
// implementation shipped here. Loki is single-tenant per request, so every
// method takes an explicit tenant.
type LokiClient interface {
	// QueryRange reads log lines for the selector over [start, end] for one
	// tenant, newest-first, up to limit lines. hitLimit reports whether the limit
	// was reached (so the caller can flag the history truncated).
	QueryRange(ctx context.Context, tenant, query string, start, end time.Time, limit int) (entries []LokiEntry, hitLimit bool, err error)
	// Tail opens a live tail for one tenant and returns a channel of tail
	// messages. The channel is closed when the tail ends (context cancellation or
	// an upstream error); a closed channel is a disconnect the caller must surface,
	// never a healthy idle feed.
	Tail(ctx context.Context, tenant, query string) (<-chan TailMessage, error)
}

// LokiEntry is one log line read from Loki for a given tenant.
type LokiEntry struct {
	Tenant      string
	TimestampNs int64
	Line        string
}

// TailMessage is one live tail delivery: a batch of entries plus any
// dropped-entry count Loki reported for the gap, so the stream can surface drops
// (E2E-LOG-06).
type TailMessage struct {
	Entries        []LokiEntry
	DroppedEntries int
}

// lokiQueryLimit bounds a single query_range request. It is a safety cap on the
// line count the BFF pulls per tenant per sweep.
const lokiQueryLimit = 5000

// lokiAdapter is the only LokiClient implementation. It talks to the central
// Loki receiver's HTTP API for history and its WebSocket tail for live updates.
// Every endpoint and tenant arrives via config (the fleet-identity firewall);
// nothing fleet-identifying is compiled in.
type lokiAdapter struct {
	base      string
	tokenFile string
	insecure  bool
	client    *http.Client
	logger    *slog.Logger
}

func newLokiAdapter(c *config.Config) *lokiAdapter {
	tr := &http.Transport{}
	if c.E2ELokiInsecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed in-cluster endpoints
	}
	return &lokiAdapter{
		base:      strings.TrimRight(c.E2ELokiURL, "/"),
		tokenFile: c.E2ELokiTokenFile,
		insecure:  c.E2ELokiInsecure,
		client:    &http.Client{Timeout: 30 * time.Second, Transport: tr},
		logger:    slog.Default(),
	}
}

// bearer reads the token file fresh (ESO rotation) and returns the header value,
// or empty when no token file is configured.
func (l *lokiAdapter) bearer() (string, error) {
	if l.tokenFile == "" {
		return "", nil
	}
	tok, err := os.ReadFile(l.tokenFile)
	if err != nil {
		return "", fmt.Errorf("read loki token: %w", err)
	}
	return "Bearer " + strings.TrimSpace(string(tok)), nil
}

// lokiQueryRangeResponse is the Loki /loki/api/v1/query_range streams response.
type lokiQueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Result     []lokiStream `json:"result"`
	} `json:"data"`
}

// lokiStream is one stream (a label set) with its [timestampNs, line] values.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func (l *lokiAdapter) QueryRange(ctx context.Context, tenant, query string, start, end time.Time, limit int) ([]LokiEntry, bool, error) {
	q := url.Values{
		"query":     {query},
		"start":     {strconv.FormatInt(start.UnixNano(), 10)},
		"end":       {strconv.FormatInt(end.UnixNano(), 10)},
		"limit":     {strconv.Itoa(limit)},
		"direction": {"backward"},
	}
	u := l.base + "/loki/api/v1/query_range?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("X-Scope-OrgID", tenant)
	bearer, err := l.bearer()
	if err != nil {
		return nil, false, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("loki query_range for tenant %s failed: %s", tenant, resp.Status)
	}
	var lr lokiQueryRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, false, fmt.Errorf("decode loki response: %w", err)
	}
	var entries []LokiEntry
	for _, s := range lr.Data.Result {
		for _, v := range s.Values {
			ts, _ := strconv.ParseInt(v[0], 10, 64)
			entries = append(entries, LokiEntry{Tenant: tenant, TimestampNs: ts, Line: v[1]})
		}
	}
	return entries, len(entries) >= limit, nil
}

// lokiTailResponse is the Loki /loki/api/v1/tail WebSocket message.
type lokiTailResponse struct {
	Streams        []lokiStream `json:"streams"`
	DroppedEntries []struct {
		Timestamp string `json:"timestamp"`
	} `json:"dropped_entries"`
}

func (l *lokiAdapter) Tail(ctx context.Context, tenant, query string) (<-chan TailMessage, error) {
	scheme := "ws"
	base := l.base
	switch {
	case strings.HasPrefix(base, "https://"):
		scheme, base = "wss", strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		scheme, base = "ws", strings.TrimPrefix(base, "http://")
	}
	q := url.Values{"query": {query}, "limit": {strconv.Itoa(lokiTailLimit)}}
	wsURL := scheme + "://" + base + "/loki/api/v1/tail?" + q.Encode()

	cfg, err := websocket.NewConfig(wsURL, "http://localhost/")
	if err != nil {
		return nil, fmt.Errorf("build loki tail config: %w", err)
	}
	// Bound the dial so a slow or black-holed Loki endpoint cannot hang the
	// subscriber indefinitely (the broker also dials off the shared lock).
	cfg.Dialer = &net.Dialer{Timeout: lokiDialTimeout}
	cfg.Header.Set("X-Scope-OrgID", tenant)
	bearer, err := l.bearer()
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		cfg.Header.Set("Authorization", bearer)
	}
	if l.insecure {
		cfg.TlsConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed in-cluster endpoints
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dial loki tail for tenant %s: %w", tenant, err)
	}

	out := make(chan TailMessage, tailChannelBuffer)
	// Close the connection when the caller's context ends so the read loop unblocks.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	go func() {
		defer close(out)
		defer func() { _ = conn.Close() }()
		for {
			var msg lokiTailResponse
			if err := websocket.JSON.Receive(conn, &msg); err != nil {
				// A receive error is a disconnect (including ctx-driven close); end the
				// tail by closing out so the broker surfaces the disconnect.
				return
			}
			tm := TailMessage{DroppedEntries: len(msg.DroppedEntries)}
			for _, s := range msg.Streams {
				for _, v := range s.Values {
					ts, _ := strconv.ParseInt(v[0], 10, 64)
					tm.Entries = append(tm.Entries, LokiEntry{Tenant: tenant, TimestampNs: ts, Line: v[1]})
				}
			}
			select {
			case out <- tm:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// lokiTailLimit bounds how many backlog lines Loki replays when a tail opens.
const lokiTailLimit = 100

// lokiDialTimeout bounds the websocket tail connection establishment.
const lokiDialTimeout = 10 * time.Second

// tailChannelBuffer bounds the per-tail delivery buffer so a slow consumer
// applies backpressure rather than growing memory unbounded (E2E-LOG-05).
const tailChannelBuffer = 64
