package electrum

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/satoshi-redline/redline/internal/domain"
)

func headerHex(timestamp uint32) string {
	header := make([]byte, 80)
	header[0] = 1
	binary.LittleEndian.PutUint32(header[68:72], timestamp)
	return hex.EncodeToString(header)
}

func fakeEndpoint(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			defer connection.Close()
			serve(connection)
		}
	}()
	return "tcp://" + listener.Addr().String()
}

func readRequest(reader *bufio.Reader) (map[string]json.RawMessage, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var request map[string]json.RawMessage
	if err = json.Unmarshal(line, &request); err != nil {
		return nil, err
	}
	return request, nil
}

func writeResponse(connection net.Conn, id int, result string) {
	_, _ = fmt.Fprintf(connection, `{"jsonrpc":"2.0","result":%s,"id":%d}`+"\n", result, id)
}

func TestCollectHandlesFragmentationNotificationsAndEOF(t *testing.T) {
	timestamp := uint32(1_800_000_000)
	endpoint := fakeEndpoint(t, func(connection net.Conn) {
		reader := bufio.NewReader(connection)
		first, err := readRequest(reader)
		if err != nil {
			return
		}
		if len(first) != 4 || first["jsonrpc"] == nil || first["method"] == nil || first["params"] == nil || first["id"] == nil || first["Method"] != nil {
			return
		}
		response := `{"jsonrpc":"2.0","result":["electrs CANARY_BANNER","1.4.3"],"id":1}` + "\n"
		_, _ = connection.Write([]byte(response[:17]))
		_, _ = connection.Write([]byte(response[17:]))
		if _, err = readRequest(reader); err != nil {
			return
		}
		notification := `{"jsonrpc":"2.0","method":"blockchain.headers.subscribe","params":[]}` + "\n"
		response = fmt.Sprintf(`{"jsonrpc":"2.0","result":{"height":123,"hex":"%s"},"id":2}`, headerHex(timestamp))
		_, _ = connection.Write([]byte(notification + response))
	})
	client, err := New(endpoint, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, reason := client.Collect(context.Background())
	if reason != "" {
		t.Fatal(reason)
	}
	if !result.Transport.Reached || result.Transport.ProtocolNegotiation != domain.ProtocolAccepted || result.Identity == nil || result.Identity.Family != domain.ElectrumElectrs || result.Tip == nil || result.Tip.Height != 123 || result.Tip.HeaderTime.Unix() != int64(timestamp) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if strings.Contains(fmt.Sprintf("%+v", result), "CANARY_BANNER") {
		t.Fatal("raw banner retained")
	}
}

func TestImplementationFamilyNormalization(t *testing.T) {
	for banner, family := range map[string]domain.ElectrumImplementationFamily{
		"electrs 0.10": domain.ElectrumElectrs,
		"Fulcrum 1.11": domain.ElectrumFulcrum,
		"other":        domain.ElectrumUnknown,
	} {
		t.Run(string(family), func(t *testing.T) {
			endpoint := fakeEndpoint(t, func(connection net.Conn) {
				reader := bufio.NewReader(connection)
				_, _ = readRequest(reader)
				writeResponse(connection, 1, fmt.Sprintf(`[%q,"1.4"]`, banner))
				_, _ = readRequest(reader)
				writeResponse(connection, 2, fmt.Sprintf(`{"height":1,"hex":%q}`, headerHex(1_800_000_000)))
			})
			client, _ := New(endpoint, time.Second)
			result, reason := client.Collect(context.Background())
			if reason != "" || result.Identity == nil || result.Identity.Family != family {
				t.Fatalf("%s: %+v %s", banner, result, reason)
			}
		})
	}
}

func TestProtocolFailuresAreTypedAndBounded(t *testing.T) {
	tests := []struct {
		name     string
		response string
		reason   domain.UnknownReasonCode
	}{
		{"malformed", "{CANARY_MALFORMED}\n", domain.InvalidResponse},
		{"invalid utf8", string([]byte{0xff, '\n'}), domain.InvalidResponse},
		{"wrong id", `{"jsonrpc":"2.0","result":["electrs","1.4"],"id":9}` + "\n", domain.InvalidResponse},
		{"rpc error", `{"jsonrpc":"2.0","error":{"code":1,"message":"CANARY_RPC_ERROR"},"id":1}` + "\n", domain.RPCError},
		{"missing result", `{"jsonrpc":"2.0","id":1}` + "\n", domain.InvalidResponse},
		{"unsupported", `{"jsonrpc":"2.0","result":["electrs","1.5"],"id":1}` + "\n", domain.UnsupportedProtocol},
		{"oversized", strings.Repeat("x", maxLineBytes+1) + "\n", domain.InvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint := fakeEndpoint(t, func(connection net.Conn) {
				_, _ = readRequest(bufio.NewReader(connection))
				_, _ = connection.Write([]byte(test.response))
			})
			client, _ := New(endpoint, time.Second)
			result, reason := client.Collect(context.Background())
			if reason != test.reason || !result.Transport.Reached {
				t.Fatalf("reason=%s result=%+v", reason, result)
			}
			rendered := fmt.Sprintf("%+v %s", result, reason)
			for _, canary := range []string{"CANARY_MALFORMED", "CANARY_RPC_ERROR"} {
				if strings.Contains(rendered, canary) {
					t.Fatal("external content escaped normalization")
				}
			}
		})
	}
}

func TestConnectionTimeoutRefusalAndCancellation(t *testing.T) {
	timeoutEndpoint := fakeEndpoint(t, func(connection net.Conn) {
		_, _ = readRequest(bufio.NewReader(connection))
		time.Sleep(100 * time.Millisecond)
	})
	client, _ := New(timeoutEndpoint, 20*time.Millisecond)
	if _, reason := client.Collect(context.Background()); reason != domain.Timeout {
		t.Fatalf("timeout classified as %s", reason)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	client, _ = New("tcp://"+address, time.Second)
	result, reason := client.Collect(context.Background())
	if reason != domain.ConnectionRefused || result.Transport.Reached {
		t.Fatalf("refusal classified as %s %+v", reason, result)
	}

	contextValue, cancel := context.WithCancel(context.Background())
	cancel()
	if _, reason = client.Collect(contextValue); reason != domain.Cancelled {
		t.Fatalf("cancellation classified as %s", reason)
	}
}

func TestTLSFailureProvesDestinationReachability(t *testing.T) {
	endpoint := fakeEndpoint(t, func(connection net.Conn) {
		_, _ = connection.Write([]byte("CANARY_TLS_ERROR\n"))
	})
	client, err := New(strings.Replace(endpoint, "tcp://", "ssl://", 1), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, reason := client.Collect(context.Background())
	if reason != domain.TLSFailure || !result.Transport.Reached || result.Transport.TLSVerification != domain.TLSVerificationFailed {
		t.Fatalf("unexpected TLS result: %+v %s", result, reason)
	}
	if strings.Contains(fmt.Sprintf("%+v", result), "CANARY_TLS_ERROR") {
		t.Fatal("TLS external data retained")
	}
}

func TestEndpointValidationRejectsUnsafeForms(t *testing.T) {
	for _, endpoint := range []string{
		"", "host:50001", "http://host:50001", "tcp://user:CANARY_USERINFO@host:50001",
		"tcp://host:50001/path", "tcp://host:50001?q=1", "tcp://host:50001#fragment", "tcp://host",
	} {
		if _, err := New(endpoint, time.Second); err == nil {
			t.Fatalf("unsafe endpoint accepted: %q", endpoint)
		} else if strings.Contains(err.Error(), "CANARY_USERINFO") {
			t.Fatal("endpoint userinfo leaked")
		}
	}
}

func TestTipParsingRejectsInvalidHeaders(t *testing.T) {
	for name, result := range map[string]string{
		"negative height": `{"height":-1,"hex":"` + headerHex(1_800_000_000) + `"}`,
		"short header":    `{"height":1,"hex":"00"}`,
		"invalid hex":     `{"height":1,"hex":"` + strings.Repeat("z", 160) + `"}`,
		"zero timestamp":  `{"height":1,"hex":"` + headerHex(0) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := fakeEndpoint(t, func(connection net.Conn) {
				reader := bufio.NewReader(connection)
				_, _ = readRequest(reader)
				writeResponse(connection, 1, `["electrs","1.4"]`)
				_, _ = readRequest(reader)
				writeResponse(connection, 2, result)
			})
			client, _ := New(endpoint, time.Second)
			_, reason := client.Collect(context.Background())
			if reason != domain.InvalidResponse {
				t.Fatalf("invalid header classified as %s", reason)
			}
		})
	}
}
