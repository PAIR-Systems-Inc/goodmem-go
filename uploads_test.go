package goodmem_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func writeTempFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

// memories.CreateFromFile uploads the file as a multipart `file` part plus a JSON
// `request` part (spaceId + contentType, no in-body content source).
func TestMemoriesCreateFromFile(t *testing.T) {
	fileBytes := []byte("%PDF-1.4 fake pdf bytes")
	path := writeTempFile(t, "doc.pdf", fileBytes)

	var gotFilename, gotFileMIME, gotRequestJSON string
	var gotFileBytes []byte
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/memories" || r.Method != "POST" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q, want multipart/form-data", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if fhs := r.MultipartForm.File["file"]; len(fhs) == 1 {
			gotFilename = fhs[0].Filename
			gotFileMIME = fhs[0].Header.Get("Content-Type")
			f, _ := fhs[0].Open()
			gotFileBytes, _ = io.ReadAll(f)
		} else {
			t.Errorf("expected exactly one `file` part, got %d", len(fhs))
		}
		if vs := r.MultipartForm.Value["request"]; len(vs) == 1 {
			gotRequestJSON = vs[0]
		} else {
			t.Errorf("expected exactly one `request` part, got %d", len(vs))
		}
		_ = json.NewEncoder(w).Encode(models.Memory{MemoryID: "mem-1", SpaceID: "sp-1"})
	})

	mem, err := c.Memories().CreateFromFile(context.Background(), path, &models.JSONMemoryCreationRequest{
		SpaceID:     "sp-1",
		ContentType: "application/pdf",
		// A stray content source must be dropped from the request part (file is the content).
		OriginalContent: strp("ignored"),
	})
	if err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}
	if mem.MemoryID != "mem-1" {
		t.Errorf("MemoryID = %q, want mem-1", mem.MemoryID)
	}
	if gotFilename != "doc.pdf" {
		t.Errorf("file part filename = %q, want doc.pdf", gotFilename)
	}
	if gotFileMIME != "application/pdf" {
		t.Errorf("file part Content-Type = %q, want application/pdf", gotFileMIME)
	}
	if string(gotFileBytes) != string(fileBytes) {
		t.Errorf("uploaded file bytes mismatch")
	}
	// The request part carries spaceId + contentType but NOT the content source.
	if !strings.Contains(gotRequestJSON, `"spaceId":"sp-1"`) || !strings.Contains(gotRequestJSON, `"contentType":"application/pdf"`) {
		t.Errorf("request part = %s, want spaceId + contentType", gotRequestJSON)
	}
	if strings.Contains(gotRequestJSON, "originalContent") {
		t.Errorf("request part must not carry a content source, got %s", gotRequestJSON)
	}
}

func TestMemoriesCreateFromFileInfersContentType(t *testing.T) {
	path := writeTempFile(t, "notes.txt", []byte("plain text"))
	var gotMIME string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		if fhs := r.MultipartForm.File["file"]; len(fhs) == 1 {
			gotMIME = fhs[0].Header.Get("Content-Type")
		}
		_ = json.NewEncoder(w).Encode(models.Memory{MemoryID: "m"})
	})
	// No ContentType set → inferred from the .txt extension.
	if _, err := c.Memories().CreateFromFile(context.Background(), path, &models.JSONMemoryCreationRequest{SpaceID: "s"}); err != nil {
		t.Fatalf("CreateFromFile: %v", err)
	}
	if !strings.HasPrefix(gotMIME, "text/plain") {
		t.Errorf("inferred content type = %q, want text/plain prefix", gotMIME)
	}
}

// CreateFromReader streams an arbitrary reader as the multipart file part and
// infers the MIME type from the filename argument.
func TestMemoriesCreateFromReader(t *testing.T) {
	body := []byte("streamed body bytes, not a file on disk")
	var gotFilename, gotMIME, gotRequestJSON string
	var gotBytes []byte
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q, want multipart/form-data", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if fhs := r.MultipartForm.File["file"]; len(fhs) == 1 {
			gotFilename = fhs[0].Filename
			gotMIME = fhs[0].Header.Get("Content-Type")
			f, _ := fhs[0].Open()
			gotBytes, _ = io.ReadAll(f)
		} else {
			t.Errorf("expected one file part, got %d", len(fhs))
		}
		if vs := r.MultipartForm.Value["request"]; len(vs) == 1 {
			gotRequestJSON = vs[0]
		}
		_ = json.NewEncoder(w).Encode(models.Memory{MemoryID: "mem-r"})
	})

	mem, err := c.Memories().CreateFromReader(context.Background(), bytes.NewReader(body), "notes.txt",
		&models.JSONMemoryCreationRequest{SpaceID: "sp-1", OriginalContent: strp("ignored")})
	if err != nil {
		t.Fatalf("CreateFromReader: %v", err)
	}
	if mem.MemoryID != "mem-r" {
		t.Errorf("MemoryID = %q, want mem-r", mem.MemoryID)
	}
	if gotFilename != "notes.txt" {
		t.Errorf("filename = %q, want notes.txt", gotFilename)
	}
	if !strings.HasPrefix(gotMIME, "text/plain") {
		t.Errorf("inferred MIME = %q, want text/plain prefix", gotMIME)
	}
	if string(gotBytes) != string(body) {
		t.Errorf("streamed bytes mismatch: %q", gotBytes)
	}
	if !strings.Contains(gotRequestJSON, `"spaceId":"sp-1"`) {
		t.Errorf("request part = %s, want spaceId", gotRequestJSON)
	}
	if strings.Contains(gotRequestJSON, "originalContent") {
		t.Errorf("request part must not carry a content source, got %s", gotRequestJSON)
	}
}

func TestMemoriesCreateFromReaderValidation(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx := context.Background()
	if _, err := c.Memories().CreateFromReader(ctx, bytes.NewReader(nil), "f.txt", nil); err == nil {
		t.Error("expected error for nil req")
	}
	if _, err := c.Memories().CreateFromReader(ctx, bytes.NewReader(nil), "f.txt", &models.JSONMemoryCreationRequest{}); err == nil {
		t.Error("expected error for empty SpaceID")
	}
	if _, err := c.Memories().CreateFromReader(ctx, nil, "f.txt", &models.JSONMemoryCreationRequest{SpaceID: "s"}); err == nil {
		t.Error("expected error for nil reader")
	}
}

func TestMemoriesCreateFromFileValidation(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx := context.Background()
	if _, err := c.Memories().CreateFromFile(ctx, "", &models.JSONMemoryCreationRequest{SpaceID: "s"}); err == nil {
		t.Error("expected error for empty filePath")
	}
	if _, err := c.Memories().CreateFromFile(ctx, "x", nil); err == nil {
		t.Error("expected error for nil req")
	}
	if _, err := c.Memories().CreateFromFile(ctx, "/no/such/file.txt", &models.JSONMemoryCreationRequest{SpaceID: "s"}); err == nil {
		t.Error("expected error for missing file")
	}
}

// ocr.DocumentFromFile reads the file and sends it as base64 in the normal JSON body
// (no multipart; the server auto-detects the format).
func TestOCRDocumentFromFile(t *testing.T) {
	fileBytes := []byte("\x89PNG fake image")
	path := writeTempFile(t, "scan.png", fileBytes)

	var gotBody map[string]any
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ocr:document" {
			t.Errorf("path = %q, want /v1/ocr:document", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json (not multipart)", ct)
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &gotBody)
		_ = json.NewEncoder(w).Encode(models.OCRDocumentResponse{PageCount: 1})
	})

	resp, err := c.OCR().DocumentFromFile(context.Background(), path, &models.OCRDocumentRequest{IncludeMarkdown: boolp(true)})
	if err != nil {
		t.Fatalf("DocumentFromFile: %v", err)
	}
	if resp.PageCount != 1 {
		t.Errorf("PageCount = %d, want 1", resp.PageCount)
	}
	if got, want := gotBody["content"], base64.StdEncoding.EncodeToString(fileBytes); got != want {
		t.Errorf("content = %v, want base64 of file (%s)", got, want)
	}
	// Preserved request field rides along; format is left for server auto-detect.
	if gotBody["includeMarkdown"] != true {
		t.Errorf("includeMarkdown = %v, want true", gotBody["includeMarkdown"])
	}
	if _, hasFormat := gotBody["format"]; hasFormat {
		t.Errorf("format should be unset (server auto-detects), got %v", gotBody["format"])
	}
}

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }
