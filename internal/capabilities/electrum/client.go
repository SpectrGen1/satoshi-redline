// Package electrum provides destination-bound, bounded Electrum protocol access.
package electrum

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/satoshi-redline/redline/internal/domain"
)

const maxLineBytes = 64 << 10

type Transport struct {
	Reached             bool
	Kind                domain.ElectrumTransportKind
	TLSVerification     domain.TLSVerificationState
	ProtocolNegotiation domain.ProtocolNegotiationState
	Category            string
}

type Identity struct {
	Family          domain.ElectrumImplementationFamily
	ProtocolVersion string
}

type Tip struct {
	Height     int64
	BlockHash  string
	HeaderTime time.Time
}

type Result struct {
	Transport Transport
	Identity  *Identity
	Tip       *Tip
}

type Client struct {
	address    string
	serverName string
	kind       domain.ElectrumTransportKind
	timeout    time.Duration
}

func New(endpoint string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || timeout <= 0 {
		return nil, fmt.Errorf("invalid Electrum configuration")
	}
	kind := domain.ElectrumTransportKind("")
	switch parsed.Scheme {
	case "tcp":
		kind = domain.ElectrumTransportTCP
	case "ssl":
		kind = domain.ElectrumTransportTLS
	default:
		return nil, fmt.Errorf("invalid Electrum configuration")
	}
	host := parsed.Hostname()
	if host == "" || parsed.Port() == "" {
		return nil, fmt.Errorf("invalid Electrum configuration")
	}
	return &Client{address: parsed.Host, serverName: host, kind: kind, timeout: timeout}, nil
}

func EndpointSideEffect(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "lan_connect"
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip != nil && ip.IsLoopback() {
		return "local_connect"
	}
	return "lan_connect"
}

func (c *Client) Collect(ctx context.Context) (Result, domain.UnknownReasonCode) {
	result := Result{Transport: Transport{
		Kind: c.kind, ProtocolNegotiation: domain.ProtocolUnknown, Category: string(domain.ProtocolError),
		TLSVerification: domain.TLSNotApplicable,
	}}
	if c.kind == domain.ElectrumTransportTLS {
		result.Transport.TLSVerification = domain.TLSVerificationUnknown
	}
	dialer := net.Dialer{Timeout: c.timeout}
	raw, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		reason := networkReason(ctx, err)
		result.Transport.Category = string(reason)
		return result, reason
	}
	result.Transport.Reached = true
	connection := raw
	stopCancellation := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stopCancellation()
	defer raw.Close()
	if err = raw.SetDeadline(operationDeadline(ctx, c.timeout)); err != nil {
		result.Transport.Category = string(domain.ProtocolError)
		return result, domain.ProtocolError
	}
	if c.kind == domain.ElectrumTransportTLS {
		tlsConnection := tls.Client(raw, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.serverName})
		if err = tlsConnection.HandshakeContext(ctx); err != nil {
			if ctx.Err() != nil {
				reason := networkReason(ctx, err)
				result.Transport.Category = string(reason)
				return result, reason
			}
			result.Transport.TLSVerification = domain.TLSVerificationFailed
			result.Transport.Category = string(domain.TLSFailure)
			return result, domain.TLSFailure
		}
		result.Transport.TLSVerification = domain.TLSVerified
		connection = tlsConnection
	}
	reader := bufio.NewReaderSize(connection, 4096)
	versionRaw, reason := exchange(ctx, connection, reader, c.timeout, 1, "server.version", []any{"SatoshiRedline/0.1", []string{"1.4", "1.4.3"}})
	if reason != "" {
		result.Transport.Category = string(reason)
		return result, reason
	}
	identity, supported, reason := parseVersion(versionRaw)
	if reason != "" {
		result.Transport.Category = string(reason)
		return result, reason
	}
	result.Identity = &identity
	if !supported {
		result.Transport.ProtocolNegotiation = domain.ProtocolUnsupported
		result.Transport.Category = string(domain.UnsupportedProtocol)
		return result, domain.UnsupportedProtocol
	}
	result.Transport.ProtocolNegotiation = domain.ProtocolAccepted
	headerRaw, reason := exchange(ctx, connection, reader, c.timeout, 2, "blockchain.headers.subscribe", []any{})
	if reason != "" {
		result.Transport.Category = string(reason)
		return result, reason
	}
	tip, reason := parseTip(headerRaw)
	if reason != "" {
		result.Transport.Category = string(reason)
		return result, reason
	}
	result.Tip = &tip
	result.Transport.Category = "valid_protocol_response"
	return result, ""
}

func operationDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int    `json:"id"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code int `json:"code"`
	} `json:"error"`
	ID *int `json:"id"`
}

func exchange(ctx context.Context, connection net.Conn, reader *bufio.Reader, timeout time.Duration, id int, method string, params any) (json.RawMessage, domain.UnknownReasonCode) {
	body, err := json.Marshal(request{JSONRPC: "2.0", Method: method, Params: params, ID: id})
	if err != nil || len(body)+1 > maxLineBytes {
		return nil, domain.ProtocolError
	}
	body = append(body, '\n')
	if err = connection.SetWriteDeadline(operationDeadline(ctx, timeout)); err != nil {
		return nil, domain.ProtocolError
	}
	written, err := connection.Write(body)
	if err != nil {
		return nil, networkReason(ctx, err)
	}
	if written != len(body) {
		return nil, domain.ProtocolError
	}
	for {
		if err = connection.SetReadDeadline(operationDeadline(ctx, timeout)); err != nil {
			return nil, domain.ProtocolError
		}
		line, readReason := readLine(reader)
		if readReason != "" {
			return nil, readReason
		}
		var envelope response
		if err = json.Unmarshal(bytes.TrimSpace(line), &envelope); err != nil || envelope.JSONRPC != "2.0" {
			return nil, domain.InvalidResponse
		}
		if envelope.ID == nil {
			continue
		}
		if *envelope.ID != id {
			return nil, domain.InvalidResponse
		}
		if envelope.Error != nil {
			return nil, domain.RPCError
		}
		if len(envelope.Result) == 0 || bytes.Equal(envelope.Result, []byte("null")) {
			return nil, domain.InvalidResponse
		}
		return envelope.Result, ""
	}
}

func readLine(reader *bufio.Reader) ([]byte, domain.UnknownReasonCode) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxLineBytes {
			return nil, domain.InvalidResponse
		}
		line = append(line, part...)
		switch {
		case err == nil:
			if !utf8.Valid(line) {
				return nil, domain.InvalidResponse
			}
			return line, ""
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			if !utf8.Valid(line) {
				return nil, domain.InvalidResponse
			}
			return line, ""
		default:
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return nil, domain.Timeout
			}
			return nil, domain.ProtocolError
		}
	}
}

func parseVersion(raw json.RawMessage) (Identity, bool, domain.UnknownReasonCode) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || len(values) != 2 {
		return Identity{}, false, domain.InvalidResponse
	}
	var banner, version string
	if json.Unmarshal(values[0], &banner) != nil || json.Unmarshal(values[1], &version) != nil || banner == "" || version == "" {
		return Identity{}, false, domain.InvalidResponse
	}
	family := domain.ElectrumUnknown
	lower := strings.ToLower(banner)
	switch {
	case strings.Contains(lower, "electrs"):
		family = domain.ElectrumElectrs
	case strings.Contains(lower, "fulcrum"):
		family = domain.ElectrumFulcrum
	}
	supported := version == "1.4" || version == "1.4.1" || version == "1.4.2" || version == "1.4.3"
	if !supported {
		return Identity{Family: family, ProtocolVersion: version}, false, ""
	}
	return Identity{Family: family, ProtocolVersion: version}, true, ""
}

func parseTip(raw json.RawMessage) (Tip, domain.UnknownReasonCode) {
	var value struct {
		Height *int64  `json:"height"`
		Hex    *string `json:"hex"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Height == nil || value.Hex == nil || *value.Height < 0 || len(*value.Hex) != 160 {
		return Tip{}, domain.InvalidResponse
	}
	header, err := hex.DecodeString(*value.Hex)
	if err != nil || len(header) != 80 {
		return Tip{}, domain.InvalidResponse
	}
	seconds := binary.LittleEndian.Uint32(header[68:72])
	if seconds == 0 {
		return Tip{}, domain.InvalidResponse
	}
	first := sha256.Sum256(header)
	second := sha256.Sum256(first[:])
	for left, right := 0, len(second)-1; left < right; left, right = left+1, right-1 {
		second[left], second[right] = second[right], second[left]
	}
	return Tip{Height: *value.Height, BlockHash: hex.EncodeToString(second[:]), HeaderTime: time.Unix(int64(seconds), 0).UTC()}, ""
}

func networkReason(ctx context.Context, err error) domain.UnknownReasonCode {
	if errors.Is(ctx.Err(), context.Canceled) {
		return domain.Cancelled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return domain.Timeout
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return domain.Timeout
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return domain.ConnectionRefused
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return domain.RouteUnreachable
	}
	return domain.ProtocolError
}
