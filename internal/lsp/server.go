package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hjseo/siba/internal/ast"
	"github.com/hjseo/siba/internal/parser"
	"github.com/hjseo/siba/internal/render"
	"github.com/hjseo/siba/internal/validate"
	"github.com/hjseo/siba/internal/workspace"
)

// Server is the SIBA LSP server
type Server struct {
	transport *Transport
	logger    *log.Logger

	mu        sync.Mutex
	rootURI   string
	workspace *workspace.Workspace
	documents map[string]string // uri → current source text

	shutdown bool
}

// NewServer creates a new LSP server
func NewServer(r io.Reader, w io.Writer, logFile string) *Server {
	var logger *log.Logger
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logger = log.New(io.Discard, "", 0)
		} else {
			logger = log.New(f, "[siba-lsp] ", log.LstdFlags)
		}
	} else {
		logger = log.New(io.Discard, "", 0)
	}

	return &Server{
		transport: NewTransport(r, w),
		logger:    logger,
		documents: make(map[string]string),
	}
}

// Run starts the server loop
func (s *Server) Run() error {
	s.logger.Println("server started")

	for {
		msg, err := s.transport.ReadMessage()
		if err != nil {
			if err == io.EOF {
				s.logger.Println("connection closed")
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		if err := s.handleMessage(msg); err != nil {
			s.logger.Printf("handle error: %v", err)
		}
	}
}

func (s *Server) handleMessage(raw json.RawMessage) error {
	// parse as generic request to determine method
	var req struct {
		ID     interface{} `json:"id"`
		Method string      `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("unmarshal request: %w", err)
	}

	s.logger.Printf("← %s (id=%v)", req.Method, req.ID)

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.ID, req.Params)
	case "initialized":
		return nil // no-op
	case "shutdown":
		return s.handleShutdown(req.ID)
	case "exit":
		os.Exit(0)
		return nil
	case "textDocument/didOpen":
		return s.handleDidOpen(req.Params)
	case "textDocument/didChange":
		return s.handleDidChange(req.Params)
	case "textDocument/didSave":
		return s.handleDidSave(req.Params)
	case "textDocument/didClose":
		return s.handleDidClose(req.Params)
	case "siba/render":
		return s.handleRender(req.ID, req.Params)
	default:
		// unknown method — respond with MethodNotFound for requests, ignore notifications
		if req.ID != nil {
			return s.sendResponse(req.ID, nil, &RespError{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			})
		}
		return nil
	}
}

// --- Handlers ---

func (s *Server) handleInitialize(id interface{}, params json.RawMessage) error {
	var p InitializeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	s.mu.Lock()
	s.rootURI = p.RootURI
	s.mu.Unlock()

	s.logger.Printf("root: %s", p.RootURI)

	// try to load workspace
	if root := uriToPath(p.RootURI); root != "" {
		ws, err := workspace.LoadWorkspace(root)
		if err == nil {
			s.mu.Lock()
			s.workspace = ws
			s.mu.Unlock()
			s.logger.Printf("workspace loaded: %d documents", len(ws.DocsByPath))
		} else {
			s.logger.Printf("workspace load failed (non-fatal): %v", err)
		}
	}

	return s.sendResponse(id, InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full sync
				Save:      &SaveOptions{IncludeText: true},
			},
		},
		ServerInfo: ServerInfo{
			Name:    "siba-lsp",
			Version: "0.1.0",
		},
	}, nil)
}

func (s *Server) handleShutdown(id interface{}) error {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()
	return s.sendResponse(id, nil, nil)
}

func (s *Server) handleDidOpen(params json.RawMessage) error {
	var p DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	s.mu.Lock()
	s.documents[p.TextDocument.URI] = p.TextDocument.Text
	s.mu.Unlock()

	s.logger.Printf("opened: %s", p.TextDocument.URI)
	return s.validateAndPublish(p.TextDocument.URI, p.TextDocument.Text)
}

func (s *Server) handleDidChange(params json.RawMessage) error {
	var p DidChangeTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	if len(p.ContentChanges) == 0 {
		return nil
	}

	// full sync — last change has the full text
	text := p.ContentChanges[len(p.ContentChanges)-1].Text

	s.mu.Lock()
	s.documents[p.TextDocument.URI] = text
	s.mu.Unlock()

	return s.validateAndPublish(p.TextDocument.URI, text)
}

func (s *Server) handleDidSave(params json.RawMessage) error {
	var p DidSaveTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	if p.Text != nil {
		s.mu.Lock()
		s.documents[p.TextDocument.URI] = *p.Text
		s.mu.Unlock()
		return s.validateAndPublish(p.TextDocument.URI, *p.Text)
	}

	// no text in save — use cached
	s.mu.Lock()
	text := s.documents[p.TextDocument.URI]
	s.mu.Unlock()

	if text != "" {
		return s.validateAndPublish(p.TextDocument.URI, text)
	}
	return nil
}

func (s *Server) handleDidClose(params json.RawMessage) error {
	var p DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.documents, p.TextDocument.URI)
	s.mu.Unlock()

	s.logger.Printf("closed: %s", p.TextDocument.URI)

	// clear diagnostics
	return s.publishDiagnostics(p.TextDocument.URI, nil)
}

// RenderParams for siba/render custom request
type RenderParams struct {
	URI string `json:"uri"`
}

// RenderResult for siba/render custom request
type RenderResult struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleRender(id interface{}, params json.RawMessage) error {
	var p RenderParams
	if err := json.Unmarshal(params, &p); err != nil {
		return s.sendResponse(id, nil, &RespError{Code: -32602, Message: "invalid params"})
	}

	s.mu.Lock()
	text, ok := s.documents[p.URI]
	s.mu.Unlock()

	if !ok {
		// try reading from disk
		path := uriToPath(p.URI)
		if path == "" {
			return s.sendResponse(id, RenderResult{Error: "invalid URI"}, nil)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return s.sendResponse(id, RenderResult{Error: err.Error()}, nil)
		}
		text = string(data)
	}

	path := uriToPath(p.URI)
	relPath := s.relativePath(path)

	doc := parser.ParseDocument(relPath, text)

	// refresh workspace document if available
	s.mu.Lock()
	ws := s.workspace
	if ws != nil {
		ws.RefreshDocument(relPath, text)
	}
	s.mu.Unlock()

	output, err := render.Render(doc)
	if err != nil {
		return s.sendResponse(id, RenderResult{Error: err.Error()}, nil)
	}

	return s.sendResponse(id, RenderResult{Content: output}, nil)
}

// --- Validation + Publishing ---

func (s *Server) validateAndPublish(uri, text string) error {
	path := uriToPath(uri)
	relPath := s.relativePath(path)

	doc := parser.ParseDocument(relPath, text)

	// refresh workspace document
	s.mu.Lock()
	ws := s.workspace
	if ws != nil {
		ws.RefreshDocument(relPath, text)
	}
	s.mu.Unlock()

	// run validation
	diags := validate.ValidateDocument(doc, ws)

	// also include parser diagnostics
	diags = append(diags, doc.Diagnostics...)

	return s.publishDiagnostics(uri, convertDiagnostics(diags))
}

func (s *Server) publishDiagnostics(uri string, diags []Diagnostic) error {
	if diags == nil {
		diags = []Diagnostic{}
	}
	return s.sendNotification("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

func convertDiagnostics(astDiags []ast.Diagnostic) []Diagnostic {
	// deduplicate by code+line
	seen := make(map[string]bool)
	var result []Diagnostic

	for _, d := range astDiags {
		key := fmt.Sprintf("%s:%d:%s", d.Code, d.Range.Start.Line, d.Message)
		if seen[key] {
			continue
		}
		seen[key] = true

		severity := 1 // Error
		switch d.Severity {
		case ast.SeverityWarning:
			severity = 2
		case ast.SeverityInfo:
			severity = 3
		case ast.SeverityHint:
			severity = 4
		}

		// LSP uses 0-based lines; ast uses 1-based
		startLine := d.Range.Start.Line
		if startLine > 0 {
			startLine--
		}
		endLine := d.Range.End.Line
		if endLine > 0 {
			endLine--
		}

		result = append(result, Diagnostic{
			Range: Range{
				Start: Position{Line: startLine, Character: d.Range.Start.Column},
				End:   Position{Line: endLine, Character: d.Range.End.Column},
			},
			Severity: severity,
			Code:     d.Code,
			Source:   "siba",
			Message:  d.Message,
		})
	}
	return result
}

// --- Transport helpers ---

func (s *Server) sendResponse(id interface{}, result interface{}, respErr *RespError) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   respErr,
	}
	s.logger.Printf("→ response (id=%v)", id)
	return s.transport.WriteMessage(resp)
}

func (s *Server) sendNotification(method string, params interface{}) error {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	s.logger.Printf("→ %s", method)
	return s.transport.WriteMessage(notif)
}

// --- Utilities ---

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		u, err := url.Parse(uri)
		if err != nil {
			return ""
		}
		return u.Path
	}
	return uri
}

func (s *Server) relativePath(absPath string) string {
	s.mu.Lock()
	rootURI := s.rootURI
	s.mu.Unlock()

	root := uriToPath(rootURI)
	if root == "" {
		return filepath.Base(absPath)
	}

	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	return rel
}
