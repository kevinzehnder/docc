package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kevinzehnder/docc/internal/diag"
)

func TestServePublishesDiagnostics(t *testing.T) {
	schemaDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	source := "---\ndocc: 1\ndocument_type: letter\ndate: 2026-08-04\n---\n"
	uri := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "letter.md")}).String()

	var in bytes.Buffer
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri, "text": source},
		},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didClose", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
		},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "shutdown", "params": map[string]any{},
	})
	writeFrame(t, &in, map[string]any{"jsonrpc": "2.0", "method": "exit"})

	var out bytes.Buffer
	if err := Serve(&in, &out, Options{SchemaDir: schemaDir}); err != nil {
		t.Fatalf("Serve() error: %v", err)
	}
	messages := readFrames(t, out.Bytes())
	if len(messages) != 4 {
		t.Fatalf("got %d server messages, want 4", len(messages))
	}

	var initialized initializeResult
	if err := json.Unmarshal(messages[0]["result"], &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized.Capabilities.TextDocumentSync != 1 || initialized.Capabilities.PositionEncoding != "utf-16" {
		t.Fatalf("initialize capabilities = %+v, want full UTF-16 sync", initialized.Capabilities)
	}

	var published publishDiagnosticsParams
	if err := json.Unmarshal(messages[1]["params"], &published); err != nil {
		t.Fatal(err)
	}
	if published.URI != uri {
		t.Errorf("published URI = %q, want %q", published.URI, uri)
	}
	if len(published.Diagnostics) == 0 {
		t.Fatal("published no diagnostics")
	}
	if published.Diagnostics[0].Source != "docc" {
		t.Errorf("diagnostic source = %q, want docc", published.Diagnostics[0].Source)
	}
	if !strings.Contains(published.Diagnostics[0].Message, "Hint:") {
		t.Errorf("diagnostic message = %q, want actionable hint", published.Diagnostics[0].Message)
	}

	var cleared publishDiagnosticsParams
	if err := json.Unmarshal(messages[2]["params"], &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.URI != uri || len(cleared.Diagnostics) != 0 {
		t.Errorf("close diagnostics = %+v, want an empty set for %q", cleared, uri)
	}
}

func TestServeRechecksFullDocumentChanges(t *testing.T) {
	schemaDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "letter.md")}).String()

	var in bytes.Buffer
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri, "text": "---\ndocc: 1\ndocument_type: letter\n---\n"},
		},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didChange", "params": map[string]any{
			"textDocument":   map[string]any{"uri": uri},
			"contentChanges": []map[string]any{{"text": "not frontmatter\n"}},
		},
	})
	writeFrame(t, &in, map[string]any{"jsonrpc": "2.0", "method": "exit"})

	var out bytes.Buffer
	if err := Serve(&in, &out, Options{SchemaDir: schemaDir}); err != nil {
		t.Fatalf("Serve() error: %v", err)
	}
	messages := readFrames(t, out.Bytes())
	if len(messages) != 3 {
		t.Fatalf("got %d server messages, want 3", len(messages))
	}
	var published publishDiagnosticsParams
	if err := json.Unmarshal(messages[2]["params"], &published); err != nil {
		t.Fatal(err)
	}
	if len(published.Diagnostics) != 0 {
		t.Fatalf("changed document diagnostics = %+v, want an empty clearing set", published.Diagnostics)
	}
	if raw := string(messages[2]["params"]); strings.Contains(raw, `"diagnostics":null`) {
		t.Errorf("publish payload contains null diagnostics, want empty array: %s", raw)
	}
}

func TestServePublishesEmptyForNonDoccMarkdown(t *testing.T) {
	schemaDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	// Plain markdown, even inside a docc project directory, must not produce
	// DOC001 — it is not a docc document.
	uri := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "notes.md")}).String()

	var in bytes.Buffer
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri, "text": "just a plain markdown file\nno frontmatter here\n"},
		},
	})
	writeFrame(t, &in, map[string]any{"jsonrpc": "2.0", "method": "exit"})

	var out bytes.Buffer
	if err := Serve(&in, &out, Options{SchemaDir: schemaDir}); err != nil {
		t.Fatalf("Serve() error: %v", err)
	}
	messages := readFrames(t, out.Bytes())
	if len(messages) != 2 {
		t.Fatalf("got %d server messages, want 2 (initialize result + publish), got %d", len(messages), len(messages))
	}
	var published publishDiagnosticsParams
	if err := json.Unmarshal(messages[1]["params"], &published); err != nil {
		t.Fatal(err)
	}
	if published.URI != uri {
		t.Errorf("published URI = %q, want %q", published.URI, uri)
	}
	if len(published.Diagnostics) != 0 {
		t.Errorf("plain markdown diagnostics = %+v, want none (it is not a docc document)", published.Diagnostics)
	}
	if raw := string(messages[1]["params"]); strings.Contains(raw, `"diagnostics":null`) {
		t.Errorf("publish payload contains null diagnostics, want empty array: %s", raw)
	}
}

func TestServeIgnoresUnrelatedFrontmatter(t *testing.T) {
	schemaDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	// Hugo-style YAML frontmatter declares no docc marker: the LSP must not
	// report DOC024 — the file is simply not a docc document.
	uri := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "post.md")}).String()
	text := "---\ntitle: My Post\ndate: 2026-08-04\ntags: [a, b]\n---\n\nContent.\n"

	var in bytes.Buffer
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	})
	writeFrame(t, &in, map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri, "text": text},
		},
	})
	writeFrame(t, &in, map[string]any{"jsonrpc": "2.0", "method": "exit"})

	var out bytes.Buffer
	if err := Serve(&in, &out, Options{SchemaDir: schemaDir}); err != nil {
		t.Fatalf("Serve() error: %v", err)
	}
	messages := readFrames(t, out.Bytes())
	if len(messages) != 2 {
		t.Fatalf("got %d server messages, want 2 (initialize result + publish)", len(messages))
	}
	var published publishDiagnosticsParams
	if err := json.Unmarshal(messages[1]["params"], &published); err != nil {
		t.Fatal(err)
	}
	if len(published.Diagnostics) != 0 {
		t.Errorf("unrelated frontmatter diagnostics = %+v, want none (no docc marker)", published.Diagnostics)
	}
}

func TestDiagnosticRangeUsesUTF16(t *testing.T) {
	// "x" begins after one two-byte rune and one surrogate-pair rune.
	r := diagnosticRange("ä😀x", diag.Position{Line: 1, Col: 7, Len: 1})
	if r.Start != (position{Line: 0, Character: 3}) {
		t.Errorf("start = %+v, want line 0 character 3", r.Start)
	}
	if r.End != (position{Line: 0, Character: 4}) {
		t.Errorf("end = %+v, want line 0 character 4", r.End)
	}
}

func TestFilePathDecodesEscapedFileURI(t *testing.T) {
	got, err := filePath("file:///tmp/letter%20draft.md")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(string(filepath.Separator), "tmp", "letter draft.md")
	if got != want {
		t.Errorf("filePath() = %q, want %q", got, want)
	}
}

func writeFrame(t *testing.T, w io.Writer, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "Content-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
}

func readFrames(t *testing.T, input []byte) []map[string]json.RawMessage {
	t.Helper()
	r := bufio.NewReader(bytes.NewReader(input))
	var messages []map[string]json.RawMessage
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			return messages
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "Content-Length: ") {
			t.Fatalf("unexpected header %q", line)
		}
		length, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length: ")))
		if err != nil {
			t.Fatal(err)
		}
		blank, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if blank != "\r\n" {
			t.Fatalf("header terminator = %q, want CRLF", blank)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(r, body); err != nil {
			t.Fatal(err)
		}
		var message map[string]json.RawMessage
		if err := json.Unmarshal(body, &message); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
}
