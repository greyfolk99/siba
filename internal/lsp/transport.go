package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Transport handles JSON-RPC 2.0 over stdio with LSP Content-Length framing
type Transport struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewTransport creates a new LSP transport
func NewTransport(r io.Reader, w io.Writer) *Transport {
	return &Transport{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// ReadMessage reads one LSP message (Content-Length header + JSON body)
func (t *Transport) ReadMessage() (json.RawMessage, error) {
	contentLength := 0

	// read headers
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			break // empty line = end of headers
		}

		if strings.HasPrefix(line, "Content-Length: ") {
			val := strings.TrimPrefix(line, "Content-Length: ")
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %s", val)
			}
			contentLength = n
		}
		// ignore other headers (Content-Type, etc.)
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	// read body
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, err
	}

	return json.RawMessage(body), nil
}

// WriteMessage writes one LSP message with Content-Length framing
func (t *Transport) WriteMessage(msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(t.writer, header); err != nil {
		return err
	}
	if _, err := t.writer.Write(body); err != nil {
		return err
	}
	return nil
}
