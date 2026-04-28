package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseCurlHeaderDump_Empty(t *testing.T) {
	h := parseCurlHeaderDump(nil)
	if len(h) != 0 {
		t.Errorf("expected empty headers, got %v", h)
	}
}

func TestParseCurlHeaderDump_BasicHeaders(t *testing.T) {
	dump := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/pdf\r\nContent-Length: 7973\r\n\r\n")
	h := parseCurlHeaderDump(dump)
	if got := h.Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type: got %q, want application/pdf", got)
	}
	if got := h.Get("Content-Length"); got != "7973" {
		t.Errorf("Content-Length: got %q, want 7973", got)
	}
}

// When server sends 100 Continue, curl -D dumps multiple header blocks. Only
// the final block's headers describe the actual response body.
func TestParseCurlHeaderDump_Skips100Continue(t *testing.T) {
	dump := []byte(
		"HTTP/1.1 100 Continue\r\n" +
			"\r\n" +
			"HTTP/1.1 200 OK\r\n" +
			"Content-Type: application/json\r\n" +
			"X-Final: yes\r\n" +
			"\r\n",
	)
	h := parseCurlHeaderDump(dump)
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", got)
	}
	if got := h.Get("X-Final"); got != "yes" {
		t.Errorf("X-Final: got %q, want yes", got)
	}
}

func TestParseCurlHeaderDump_MultiValueHeader(t *testing.T) {
	dump := []byte(
		"HTTP/1.1 200 OK\r\n" +
			"Set-Cookie: a=1; Path=/\r\n" +
			"Set-Cookie: b=2; Path=/\r\n" +
			"\r\n",
	)
	h := parseCurlHeaderDump(dump)
	values := h.Values("Set-Cookie")
	if len(values) != 2 {
		t.Fatalf("expected 2 Set-Cookie values, got %d: %v", len(values), values)
	}
	if values[0] != "a=1; Path=/" || values[1] != "b=2; Path=/" {
		t.Errorf("unexpected Set-Cookie values: %v", values)
	}
}

func TestParseCurlHeaderDump_LFOnlyLineEndings(t *testing.T) {
	dump := []byte("HTTP/1.1 200 OK\nContent-Type: text/plain\n\n")
	h := parseCurlHeaderDump(dump)
	if got := h.Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type: got %q, want text/plain", got)
	}
}

func TestParseCurlHeaderDump_ColonInValue(t *testing.T) {
	dump := []byte("HTTP/1.1 200 OK\r\nLocation: https://example.com:8443/path\r\n\r\n")
	h := parseCurlHeaderDump(dump)
	if got := h.Get("Location"); got != "https://example.com:8443/path" {
		t.Errorf("Location: got %q, want full URL with port", got)
	}
}

// The whole point of the bug fix: arbitrary binary bytes in the response body
// must round-trip through executeHTTPInNetwork's post-processing untouched. By
// extracting buildHTTPResponseFromCurlFiles as a pure function, we can verify
// this without needing Docker.
func TestBuildHTTPResponseFromCurlFiles_BinaryBodyPreserved(t *testing.T) {
	// Simulate a PDF-ish payload: %PDF magic, valid UTF-8 prefix, then bytes
	// that are NOT valid UTF-8 (would be replaced with U+FFFD if anything in
	// the path went through json.Marshal of a Go string).
	body := []byte{
		'%', 'P', 'D', 'F', '-', '1', '.', '7', '\n',
		0xFF, 0xFE, 0x00, 0x80, 0xC0, 0xC1, // invalid UTF-8 sequences
		0x00, // NUL byte
		0x0A, 0x0D, 0x0A, 0x0D, // sequences that look like header/body separators
		'%', '%', 'E', 'O', 'F', '\n',
	}
	headers := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/pdf\r\n\r\n")
	stdout := []byte("200")

	resp, err := buildHTTPResponseFromCurlFiles(stdout, body, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode: got %d, want 200", resp.StatusCode)
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("body bytes differ:\n got %x\nwant %x", resp.Body, body)
	}
	if got := resp.Headers.Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type: got %q, want application/pdf", got)
	}
}

func TestBuildHTTPResponseFromCurlFiles_StatusCodeWithWhitespace(t *testing.T) {
	resp, err := buildHTTPResponseFromCurlFiles([]byte(" 404 \n"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("StatusCode: got %d, want 404", resp.StatusCode)
	}
}

func TestBuildHTTPResponseFromCurlFiles_BadStatusCode(t *testing.T) {
	_, err := buildHTTPResponseFromCurlFiles([]byte("not-a-number"), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-numeric status code")
	}
	if !strings.Contains(err.Error(), "status code") {
		t.Errorf("expected error to mention status code, got %v", err)
	}
}

func TestBuildHTTPResponseFromCurlFiles_EmptyBodyAndHeaders(t *testing.T) {
	resp, err := buildHTTPResponseFromCurlFiles([]byte("204"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("StatusCode: got %d, want 204", resp.StatusCode)
	}
	if len(resp.Body) != 0 {
		t.Errorf("expected empty body, got %d bytes", len(resp.Body))
	}
	if len(resp.Headers) != 0 {
		t.Errorf("expected empty headers, got %v", resp.Headers)
	}
}
