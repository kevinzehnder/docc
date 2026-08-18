// Package lsp exposes docc's checker through the Language Server Protocol.
//
// It intentionally implements the small, dependency-free subset needed for
// editor diagnostics: JSON-RPC over stdio, full-document synchronization, and
// textDocument/publishDiagnostics notifications.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/kevinzehnder/docc/internal/diag"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/profile"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
)

const (
	jsonRPCVersion = "2.0"
	maxMessageSize = 16 << 20
)

// Options configures a Server.
type Options struct {
	// SchemaDir overrides discovery of .docc/schemas.
	SchemaDir string
	// DocType overrides document_type in every checked document.
	DocType string
}

// Server handles one LSP connection.
type Server struct {
	in      *bufio.Reader
	out     io.Writer
	options Options
	docs    map[string]document
}

type document struct {
	path string
	text string
}

// Serve runs a Language Server Protocol connection until the client exits or
// closes the input stream. LSP traffic is written exclusively to out; callers
// may safely use stderr for logs.
func Serve(in io.Reader, out io.Writer, options Options) error {
	s := &Server{
		in:      bufio.NewReader(in),
		out:     out,
		options: options,
		docs:    map[string]document{},
	}
	return s.serve()
}

func (s *Server) serve() error {
	for {
		msg, err := readMessage(s.in)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read LSP message: %w", err)
		}
		exit, err := s.handle(msg)
		if err != nil {
			return err
		}
		if exit {
			return nil
		}
	}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func (s *Server) handle(msg request) (bool, error) {
	if msg.JSONRPC != jsonRPCVersion {
		return false, s.replyError(msg.ID, -32600, "invalid JSON-RPC version")
	}

	switch msg.Method {
	case "initialize":
		if err := s.replyResult(msg.ID, initializeResult{Capabilities: capabilities{
			PositionEncoding: "utf-16",
			TextDocumentSync: 1, // TextDocumentSyncKind.Full
		}, ServerInfo: serverInfo{Name: "docc"}}); err != nil {
			return false, err
		}
	case "initialized", "$/cancelRequest":
		// No setup is needed and checks are synchronous, so cancellation has no
		// work to interrupt.
	case "shutdown":
		if err := s.replyResult(msg.ID, nil); err != nil {
			return false, err
		}
	case "exit":
		return true, nil
	case "textDocument/didOpen":
		var params didOpenParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, s.invalidParams(msg.ID, err)
		}
		path, err := filePath(params.TextDocument.URI)
		if err != nil {
			return false, s.showError(err)
		}
		s.docs[params.TextDocument.URI] = document{path: path, text: params.TextDocument.Text}
		return false, s.publish(params.TextDocument.URI)
	case "textDocument/didChange":
		var params didChangeParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, s.invalidParams(msg.ID, err)
		}
		doc, ok := s.docs[params.TextDocument.URI]
		if !ok || len(params.ContentChanges) == 0 {
			return false, nil
		}
		// The server advertises full synchronization, so each change contains the
		// complete document text. Use the last change defensively if a client
		// sends more than one.
		doc.text = params.ContentChanges[len(params.ContentChanges)-1].Text
		s.docs[params.TextDocument.URI] = doc
		return false, s.publish(params.TextDocument.URI)
	case "textDocument/didSave":
		var params didSaveParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, s.invalidParams(msg.ID, err)
		}
		if params.Text != nil {
			path, err := filePath(params.TextDocument.URI)
			if err != nil {
				return false, s.showError(err)
			}
			s.docs[params.TextDocument.URI] = document{path: path, text: *params.Text}
		}
		if _, ok := s.docs[params.TextDocument.URI]; !ok {
			return false, nil
		}
		return false, s.publish(params.TextDocument.URI)
	case "textDocument/didClose":
		var params didCloseParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, s.invalidParams(msg.ID, err)
		}
		delete(s.docs, params.TextDocument.URI)
		return false, s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
			URI:         params.TextDocument.URI,
			Diagnostics: []lspDiagnostic{},
		})
	default:
		if len(msg.ID) != 0 {
			return false, s.replyError(msg.ID, -32601, "method not found: "+msg.Method)
		}
	}
	return false, nil
}

type capabilities struct {
	PositionEncoding string `json:"positionEncoding"`
	TextDocumentSync int    `json:"textDocumentSync"`
}

type serverInfo struct {
	Name string `json:"name"`
}

type initializeResult struct {
	Capabilities capabilities `json:"capabilities"`
	ServerInfo   serverInfo   `json:"serverInfo"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type textDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument   textDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange        `json:"contentChanges"`
}

type didSaveParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

func (s *Server) publish(uri string) error {
	doc := s.docs[uri]
	diagnostics, err := check(doc.path, []byte(doc.text), s.options)
	if err != nil {
		if clearErr := s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: uri, Diagnostics: []lspDiagnostic{}}); clearErr != nil {
			return clearErr
		}
		return s.showError(err)
	}
	return s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         uri,
		Diagnostics: lspDiagnostics(doc.text, diagnostics),
	})
}

// check keeps the LSP adapter independent of command-line flag parsing while
// sharing the compiler's parser, schema loader, and semantic passes.
//
// A document that is not actually a docc document — no `docc: <version>`
// marker in frontmatter — is silently ignored. The LSP must stay quiet beside
// regular markdown files; the checker only speaks when a file opts in via the
// docc marker. Resolution always answers something (the builtin starter pack
// at worst), so the marker is the gate.
func check(path string, source []byte, options Options) (diag.List, error) {
	schemaDir := options.SchemaDir
	if schemaDir == "" {
		paths, err := profile.XDGPaths()
		if err != nil {
			return nil, err
		}
		resolved, err := profile.Resolve(path, paths)
		if err != nil {
			return nil, err
		}
		schemaDir = resolved.SchemaDir
	}
	// A resolved directory without schemas is a valid editing state: the
	// project simply has no document types yet. Stay quiet instead of erroring.
	if _, err := os.Stat(schemaDir); err != nil {
		return nil, nil
	}
	set, err := schema.Load(schemaDir)
	if err != nil {
		return nil, err
	}
	file, parseDiagnostics := parse.Parse(path, source)
	res := sema.Check(file, set, parseDiagnostics, options.DocType)
	// No `docc: <version>` marker in frontmatter: this is a regular markdown
	// file or a file with unrelated YAML frontmatter (Hugo, Obsidian, …).
	// The LSP stays quiet unless a file opts in.
	if _, ok := res.Meta.Lookup("docc"); !ok {
		return nil, nil
	}
	return res.Diagnostics, nil
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Code     string   `json:"code"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string          `json:"uri"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
}

func lspDiagnostics(source string, diagnostics diag.List) []lspDiagnostic {
	out := make([]lspDiagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		severity := 2 // DiagnosticSeverity.Warning
		if d.Severity == diag.Error {
			severity = 1 // DiagnosticSeverity.Error
		}
		message := d.Message
		if d.Hint != "" {
			message += "\nHint: " + d.Hint
		}
		out = append(out, lspDiagnostic{
			Range:    diagnosticRange(source, d.Pos),
			Severity: severity,
			Code:     d.Code,
			Source:   "docc",
			Message:  message,
		})
	}
	return out
}

// diagnosticRange translates docc's 1-indexed byte positions to LSP's
// 0-indexed UTF-16 code-unit positions. A file-level diagnostic has no natural
// source span, so it is anchored at the start of the document.
func diagnosticRange(source string, pos diag.Position) lspRange {
	if pos.Line < 1 {
		return lspRange{}
	}
	lines := strings.Split(source, "\n")
	lineIndex := pos.Line - 1
	if lineIndex >= len(lines) {
		return lspRange{}
	}
	line := strings.TrimSuffix(lines[lineIndex], "\r")
	startByte := min(max(pos.Col-1, 0), len(line))
	start := position{Line: lineIndex, Character: utf16Column(line, startByte, false)}
	end := start
	if pos.Len > 0 {
		endByte := min(startByte+pos.Len, len(line))
		end.Character = utf16Column(line, endByte, true)
	}
	return lspRange{Start: start, End: end}
}

// utf16Column reports UTF-16 code units before byteOffset. When byteOffset is
// in the middle of a UTF-8 sequence, roundUp selects the following boundary;
// this keeps an LSP range valid even for a malformed byte span.
func utf16Column(line string, byteOffset int, roundUp bool) int {
	byteOffset = min(max(byteOffset, 0), len(line))
	units := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		if i == byteOffset {
			break
		}
		if i+size > byteOffset {
			if roundUp {
				units += utf16Units(r)
			}
			break
		}
		units += utf16Units(r)
		i += size
	}
	return units
}

func utf16Units(r rune) int {
	if utf16.RuneLen(r) == 2 {
		return 2
	}
	return 1
}

func filePath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse document URI: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("docc only supports file URIs, got %q", uri)
	}
	path, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		return "", fmt.Errorf("decode document URI: %w", err)
	}
	if u.Host != "" && u.Host != "localhost" {
		path = "//" + u.Host + path
	}
	return filepath.FromSlash(path), nil
}

func (s *Server) invalidParams(id json.RawMessage, err error) error {
	if len(id) == 0 {
		return nil
	}
	return s.replyError(id, -32602, "invalid parameters: "+err.Error())
}

func (s *Server) showError(err error) error {
	return s.notify("window/showMessage", map[string]any{
		"type":    1, // MessageType.Error
		"message": "docc: " + err.Error(),
	})
}

func (s *Server) replyResult(id json.RawMessage, result any) error {
	return s.write(map[string]any{
		"jsonrpc": jsonRPCVersion,
		"id":      json.RawMessage(id),
		"result":  result,
	})
}

func (s *Server) replyError(id json.RawMessage, code int, message string) error {
	return s.write(map[string]any{
		"jsonrpc": jsonRPCVersion,
		"id":      json.RawMessage(id),
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func (s *Server) notify(method string, params any) error {
	return s.write(map[string]any{
		"jsonrpc": jsonRPCVersion,
		"method":  method,
		"params":  params,
	})
}

func (s *Server) write(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = s.out.Write(body)
	return err
}

func readMessage(r *bufio.Reader) (request, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return request{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return request{}, fmt.Errorf("malformed header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			contentLength, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return request{}, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}
	if contentLength < 0 || contentLength > maxMessageSize {
		return request{}, fmt.Errorf("invalid Content-Length %d", contentLength)
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return request{}, err
	}
	var msg request
	if err := json.Unmarshal(body, &msg); err != nil {
		return request{}, fmt.Errorf("decode JSON-RPC message: %w", err)
	}
	return msg, nil
}
