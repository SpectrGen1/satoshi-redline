package bitcoinrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxResponse = 1 << 20

type ChainInfo struct {
	Blocks, Headers      int64
	VerificationProgress float64
	InitialBlockDownload bool
	TipTime              time.Time
}
type Client struct {
	endpoint, cookie string
	http             *http.Client
}

func New(endpoint, cookieFile, cookieEnv string, timeout time.Duration) (*Client, domain.UnknownReason) {
	if endpoint == "" {
		return nil, domain.NotConfigured
	}
	u, parseErr := url.Parse(endpoint)
	if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, domain.NotConfigured
	}
	var cookie string
	var err error
	if cookieFile != "" {
		b, e := os.ReadFile(cookieFile)
		err = e
		cookie = strings.TrimSpace(string(b))
	} else if cookieEnv != "" {
		cookie = os.Getenv(cookieEnv)
		if cookie == "" {
			err = errors.New("unavailable")
		}
	} else {
		err = errors.New("unavailable")
	}
	if err != nil || !strings.Contains(cookie, ":") {
		return nil, domain.SecretReferenceUnavailable
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true}
	return &Client{endpoint, cookie, &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, ""
}
func (c *Client) Chain(ctx context.Context) (ChainInfo, domain.UnknownReason, bool) {
	var result struct {
		Blocks               int64   `json:"blocks"`
		Headers              int64   `json:"headers"`
		VerificationProgress float64 `json:"verificationprogress"`
		InitialBlockDownload bool    `json:"initialblockdownload"`
		Time                 int64   `json:"time"`
	}
	_, reason, reachable := c.call(ctx, "getblockchaininfo", &result)
	if reason != "" {
		return ChainInfo{}, reason, reachable
	}
	if result.Blocks < 0 || result.Headers < 0 || result.VerificationProgress < 0 || result.VerificationProgress > 1 || result.Time <= 0 {
		return ChainInfo{}, domain.InvalidResponse, true
	}
	return ChainInfo{result.Blocks, result.Headers, result.VerificationProgress, result.InitialBlockDownload, time.Unix(result.Time, 0).UTC()}, "", true
}
func (c *Client) call(ctx context.Context, method string, into any) (json.RawMessage, domain.UnknownReason, bool) {
	b, _ := json.Marshal(struct {
		JSONRPC, Method string
		Params          []any
		ID              int
	}{"1.0", method, []any{}, 1})
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(b))
	if e != nil {
		return nil, domain.ProtocolError, false
	}
	p := strings.SplitN(c.cookie, ":", 2)
	req.SetBasicAuth(p[0], p[1])
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		if errors.Is(e, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, domain.Timeout, false
		}
		return nil, domain.ProtocolError, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, domain.AuthenticationRejected, true
	}
	limited := io.LimitReader(resp.Body, maxResponse+1)
	raw, e := io.ReadAll(limited)
	if e != nil || len(raw) > maxResponse {
		return nil, domain.InvalidResponse, true
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if e = json.Unmarshal(raw, &envelope); e != nil || envelope.Error != nil {
		return nil, domain.InvalidResponse, true
	}
	if e = json.Unmarshal(envelope.Result, into); e != nil {
		return nil, domain.InvalidResponse, true
	}
	return envelope.Result, "", true
}

var _ = fmt.Sprintf
