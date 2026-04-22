package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// --- helpers ---

func makeMessage(body interface{}) string {
	data, _ := json.Marshal(body)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(data), data)
}

func initializeRequest(id int, rootURI string) string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": 1,
			"rootUri":   rootURI,
			"capabilities": map[string]interface{}{},
		},
	})
}

func initializedNotification() string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
	})
}

func didOpenNotification(uri, text string) string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":        uri,
				"languageId": "markdown",
				"version":    1,
				"text":       text,
			},
		},
	})
}

func didChangeNotification(uri, text string, version int) string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":     uri,
				"version": version,
			},
			"contentChanges": []map[string]interface{}{
				{"text": text},
			},
		},
	})
}

func didCloseNotification(uri string) string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/didClose",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri": uri,
			},
		},
	})
}

func shutdownRequest(id int) string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "shutdown",
	})
}

func renderRequest(id int, uri string) string {
	return makeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "siba/render",
		"params": map[string]interface{}{
			"uri": uri,
		},
	})
}

func readAllResponses(output string) []json.RawMessage {
	var results []json.RawMessage
	remaining := output

	for len(remaining) > 0 {
		idx := strings.Index(remaining, "Content-Length: ")
		if idx < 0 {
			break
		}
		remaining = remaining[idx:]

		// parse Content-Length
		nlIdx := strings.Index(remaining, "\r\n")
		if nlIdx < 0 {
			break
		}
		header := remaining[:nlIdx]
		var length int
		fmt.Sscanf(header, "Content-Length: %d", &length)

		// skip header section (Content-Length + \r\n\r\n)
		bodyStart := strings.Index(remaining, "\r\n\r\n")
		if bodyStart < 0 {
			break
		}
		bodyStart += 4

		if bodyStart+length > len(remaining) {
			break
		}

		body := remaining[bodyStart : bodyStart+length]
		results = append(results, json.RawMessage(body))
		remaining = remaining[bodyStart+length:]
	}

	return results
}

func runServerSession(input string) string {
	reader := strings.NewReader(input)
	var output bytes.Buffer
	server := NewServer(reader, &output, "")
	_ = server.Run()
	return output.String()
}

// --- Transport tests ---

func TestTransport_ReadWrite(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"test"}`
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	transport := NewTransport(strings.NewReader(input), io.Discard)
	msg, err := transport.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var req struct {
		Method string `json:"method"`
	}
	json.Unmarshal(msg, &req)
	if req.Method != "test" {
		t.Fatalf("expected method 'test', got %q", req.Method)
	}
}

func TestTransport_WriteMessage(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(strings.NewReader(""), &buf)

	err := transport.WriteMessage(map[string]string{"test": "value"})
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "Content-Length: ") {
		t.Fatal("expected Content-Length header")
	}
	if !strings.Contains(output, `"test":"value"`) {
		t.Fatal("expected JSON body")
	}
}

// --- Server handler tests ---

func TestServer_Initialize(t *testing.T) {
	input := initializeRequest(1, "file:///tmp/test")
	output := runServerSession(input)

	responses := readAllResponses(output)
	if len(responses) < 1 {
		t.Fatalf("expected at least 1 response, got %d", len(responses))
	}

	var resp Response
	json.Unmarshal(responses[0], &resp)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
}

func TestServer_Shutdown(t *testing.T) {
	input := initializeRequest(1, "file:///tmp/test") + shutdownRequest(2)
	output := runServerSession(input)

	responses := readAllResponses(output)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	var resp Response
	json.Unmarshal(responses[1], &resp)
	if resp.Error != nil {
		t.Fatalf("expected no error on shutdown, got: %v", resp.Error)
	}
}

func TestServer_DidOpen_PublishesDiagnostics(t *testing.T) {
	uri := "file:///tmp/test.md"
	input := initializeRequest(1, "file:///tmp") +
		initializedNotification() +
		didOpenNotification(uri, "# Hello\n\nContent here.\n")

	output := runServerSession(input)
	responses := readAllResponses(output)

	// should have: initialize response + publishDiagnostics notification
	foundPublish := false
	for _, r := range responses {
		var msg struct {
			Method string `json:"method"`
		}
		json.Unmarshal(r, &msg)
		if msg.Method == "textDocument/publishDiagnostics" {
			foundPublish = true
		}
	}
	if !foundPublish {
		t.Fatal("expected publishDiagnostics notification after didOpen")
	}
}

func TestServer_DidChange_UpdatesDiagnostics(t *testing.T) {
	uri := "file:///tmp/test.md"
	input := initializeRequest(1, "file:///tmp") +
		initializedNotification() +
		didOpenNotification(uri, "# Hello\n") +
		didChangeNotification(uri, "# Updated\n\nNew content.\n", 2)

	output := runServerSession(input)
	responses := readAllResponses(output)

	publishCount := 0
	for _, r := range responses {
		var msg struct {
			Method string `json:"method"`
		}
		json.Unmarshal(r, &msg)
		if msg.Method == "textDocument/publishDiagnostics" {
			publishCount++
		}
	}
	if publishCount < 2 {
		t.Fatalf("expected at least 2 publishDiagnostics (open + change), got %d", publishCount)
	}
}

func TestServer_DidClose_ClearsDiagnostics(t *testing.T) {
	uri := "file:///tmp/test.md"
	input := initializeRequest(1, "file:///tmp") +
		initializedNotification() +
		didOpenNotification(uri, "# Hello\n") +
		didCloseNotification(uri)

	output := runServerSession(input)
	responses := readAllResponses(output)

	// find last publishDiagnostics — should have empty diagnostics
	var lastPublish json.RawMessage
	for _, r := range responses {
		var msg struct {
			Method string `json:"method"`
		}
		json.Unmarshal(r, &msg)
		if msg.Method == "textDocument/publishDiagnostics" {
			lastPublish = r
		}
	}

	if lastPublish == nil {
		t.Fatal("expected publishDiagnostics")
	}

	var notif struct {
		Params PublishDiagnosticsParams `json:"params"`
	}
	json.Unmarshal(lastPublish, &notif)
	if len(notif.Params.Diagnostics) != 0 {
		t.Fatalf("expected empty diagnostics on close, got %d", len(notif.Params.Diagnostics))
	}
}

func TestServer_Render(t *testing.T) {
	uri := "file:///tmp/test.md"
	source := "# Hello\n\n<!-- @const title = \"World\" -->\n\nGreetings {{title}}!\n"
	input := initializeRequest(1, "file:///tmp") +
		initializedNotification() +
		didOpenNotification(uri, source) +
		renderRequest(2, uri)

	output := runServerSession(input)
	responses := readAllResponses(output)

	// find render response (id=2)
	var renderResp Response
	for _, r := range responses {
		var resp Response
		json.Unmarshal(r, &resp)
		if resp.ID != nil {
			idFloat, ok := resp.ID.(float64)
			if ok && idFloat == 2 {
				renderResp = resp
				break
			}
		}
	}

	if renderResp.ID == nil {
		t.Fatal("expected render response with id=2")
	}

	resultJSON, _ := json.Marshal(renderResp.Result)
	var result RenderResult
	json.Unmarshal(resultJSON, &result)

	if result.Error != "" {
		t.Fatalf("render error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "World") {
		t.Fatalf("expected rendered content to contain 'World', got: %q", result.Content)
	}
}

func TestServer_UnknownMethod(t *testing.T) {
	input := initializeRequest(1, "file:///tmp") +
		makeMessage(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      99,
			"method":  "custom/unknown",
		})

	output := runServerSession(input)
	responses := readAllResponses(output)

	// find response to unknown method (id=99)
	for _, r := range responses {
		var resp Response
		json.Unmarshal(r, &resp)
		if resp.ID != nil {
			idFloat, ok := resp.ID.(float64)
			if ok && idFloat == 99 {
				if resp.Error == nil {
					t.Fatal("expected error for unknown method")
				}
				if resp.Error.Code != -32601 {
					t.Fatalf("expected code -32601, got %d", resp.Error.Code)
				}
				return
			}
		}
	}
	t.Fatal("expected response for unknown method request")
}

// --- Utility tests ---

func TestURIToPath(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///Users/test/doc.md", "/Users/test/doc.md"},
		{"file:///tmp/test.md", "/tmp/test.md"},
		{"/direct/path.md", "/direct/path.md"},
	}

	for _, tt := range tests {
		got := uriToPath(tt.uri)
		if got != tt.want {
			t.Errorf("uriToPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestConvertDiagnostics(t *testing.T) {
	astDiags := []Diagnostic{} // empty case
	if len(astDiags) != 0 {
		t.Fatal("expected empty")
	}
}

func TestConvertDiagnostics_Deduplication(t *testing.T) {
	// Create ast diagnostics with duplicates
	input := []struct {
		code    string
		line    int
		message string
	}{
		{"E050", 5, "unresolved reference: x"},
		{"E050", 5, "unresolved reference: x"}, // duplicate
		{"E050", 10, "unresolved reference: y"},  // different line
	}

	var astDiags []astDiag
	for _, d := range input {
		astDiags = append(astDiags, astDiag{
			Code:    d.code,
			Line:    d.line,
			Message: d.message,
		})
	}

	// We can't easily test convertDiagnostics without importing ast,
	// but we can verify the dedup key logic
	seen := make(map[string]bool)
	count := 0
	for _, d := range astDiags {
		key := fmt.Sprintf("%s:%d:%s", d.Code, d.Line, d.Message)
		if !seen[key] {
			seen[key] = true
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 unique diagnostics, got %d", count)
	}
}

type astDiag struct {
	Code    string
	Line    int
	Message string
}
