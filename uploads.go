package goodmem

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

// File-upload convenience methods — the Go analog of the Python/Java SDKs'
// file_path overloads on memories.create and ocr.document. memories.create uses a
// multipart upload (a `file` part + a JSON `request` part, the server's contract);
// ocr.document reads the file and sends it as base64 in the normal JSON body (the
// server auto-detects the document format from the bytes, so no format inference).

// inferContentTypeByExt maps a file extension to a MIME type, defaulting to
// application/octet-stream when the extension is unknown (the same fallback the
// Python SDK uses via mimetypes.guess_type).
func inferContentTypeByExt(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// doMultipartStream POSTs a memory-create multipart request: a `file` part
// carrying the content and a JSON `request` part carrying the metadata shell. The
// body is streamed through an io.Pipe so neither the file bytes nor the assembled
// multipart envelope are ever fully buffered — a large upload holds only the copy
// buffer in memory, not ~2x the file. Error handling mirrors doJSON (typed status
// errors, network/parse wrapping).
func (c *Client) doMultipartStream(ctx context.Context, method, path, filename string, file io.Reader, fileMIME string, requestPart, out any) error {
	reqJSON, err := json.Marshal(requestPart)
	if err != nil {
		return &GoodmemError{Message: "failed to serialize multipart request part: " + err.Error(), Cause: err}
	}

	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	// The boundary is fixed at construction, so capture the content type before
	// the writer goroutine starts touching w (avoids a data race on its state).
	formContentType := w.FormDataContentType()

	go func() {
		// Any failure assembling the body is propagated to the request reader via
		// CloseWithError so httpClient.Do surfaces the real cause instead of an
		// opaque short read.
		fh := textproto.MIMEHeader{}
		fh.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
		fh.Set("Content-Type", fileMIME)
		fp, err := w.CreatePart(fh)
		if err != nil {
			_ = pw.CloseWithError(&GoodmemError{Message: "failed to build multipart file part: " + err.Error(), Cause: err})
			return
		}
		if _, err := io.Copy(fp, file); err != nil {
			_ = pw.CloseWithError(&GoodmemError{Message: "failed to stream multipart file part: " + err.Error(), Cause: err})
			return
		}
		rh := textproto.MIMEHeader{}
		rh.Set("Content-Disposition", `form-data; name="request"`)
		rh.Set("Content-Type", "application/json")
		rp, err := w.CreatePart(rh)
		if err != nil {
			_ = pw.CloseWithError(&GoodmemError{Message: "failed to build multipart request part: " + err.Error(), Cause: err})
			return
		}
		if _, err := rp.Write(reqJSON); err != nil {
			_ = pw.CloseWithError(&GoodmemError{Message: "failed to write multipart request part: " + err.Error(), Cause: err})
			return
		}
		if err := w.Close(); err != nil {
			_ = pw.CloseWithError(&GoodmemError{Message: "failed to finalize multipart body: " + err.Error(), Cause: err})
			return
		}
		_ = pw.Close()
	}()

	req, err := c.newRequest(ctx, method, path, nil, pr)
	if err != nil {
		_ = pr.CloseWithError(err) // unblock the writer goroutine
		return err
	}
	req.Header.Set("Content-Type", formContentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{Message: "HTTP request failed: " + err.Error(), Cause: err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &NetworkError{Message: "failed to read response body: " + err.Error(), Cause: err}
	}
	if resp.StatusCode >= 400 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return mapError(resp.StatusCode, string(data), retryAfter)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &GoodmemError{Message: "malformed response body: " + truncate(string(data), 200), Cause: err}
	}
	return nil
}

// CreateFromFile creates a memory from a local file via a multipart upload — the Go
// analog of the Python/Java memories.create(file_path) overload. req supplies the
// space and any metadata (SpaceID is required); the file is the content, so any
// OriginalContent/OriginalContentB64 on req is ignored. When req.ContentType is
// empty it is inferred from the file extension.
func (s *MemoriesService) CreateFromFile(ctx context.Context, filePath string, req *models.JSONMemoryCreationRequest) (*models.Memory, error) {
	if req == nil {
		return nil, fmt.Errorf("memories.createFromFile: req must not be nil")
	}
	if req.SpaceID == "" {
		return nil, fmt.Errorf("memories.createFromFile: SpaceID must not be empty")
	}
	if filePath == "" {
		return nil, fmt.Errorf("memories.createFromFile: filePath must not be empty")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("memories.createFromFile: open %s: %w", filePath, err)
	}
	defer f.Close()
	// Pin the content type from the path here so CreateFromReader (which infers
	// from its filename argument) reproduces exactly CreateFromFile's behavior.
	inner := *req
	if inner.ContentType == "" {
		inner.ContentType = inferContentTypeByExt(filePath)
	}
	return s.CreateFromReader(ctx, f, filepath.Base(filePath), &inner)
}

// CreateFromReader creates a memory by streaming an arbitrary io.Reader as the
// multipart file part — the streaming sibling of CreateFromFile for content that
// isn't a local file (an object-store stream, an HTTP response body, an in-memory
// buffer). The body is streamed with io.Pipe so a large upload never stages the
// whole payload in memory. filename names the part and, when req.ContentType is
// empty, drives MIME inference from its extension (pass "" to fall back to
// application/octet-stream). req supplies the space and metadata (SpaceID
// required); any OriginalContent/OriginalContentB64 on it is ignored — the reader
// is the content.
func (s *MemoriesService) CreateFromReader(ctx context.Context, r io.Reader, filename string, req *models.JSONMemoryCreationRequest) (*models.Memory, error) {
	if req == nil {
		return nil, fmt.Errorf("memories.createFromReader: req must not be nil")
	}
	if req.SpaceID == "" {
		return nil, fmt.Errorf("memories.createFromReader: SpaceID must not be empty")
	}
	if r == nil {
		return nil, fmt.Errorf("memories.createFromReader: reader must not be nil")
	}
	contentType := req.ContentType
	if contentType == "" {
		contentType = inferContentTypeByExt(filename)
	}
	// The file part carries the content; the JSON request part must NOT also set a
	// content source (the server's multipart contract — the in-body content mutex
	// would otherwise reject it). Copy req so the caller's value is untouched.
	shell := *req
	shell.OriginalContent = nil
	shell.OriginalContentB64 = nil
	shell.ContentType = contentType

	var out models.Memory
	if err := s.client.doMultipartStream(ctx, "POST", "/v1/memories", filename, r, contentType, &shell, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DocumentFromFile runs OCR over a local file — the Go analog of the Python/Java
// ocr.document(file_path) overload. It reads the file and sends it as base64 in the
// normal JSON body; the document format is NOT inferred (the server auto-detects it
// from the bytes — set req.Format only to override). Any req.Content is replaced by
// the file's contents.
func (s *OCRService) DocumentFromFile(ctx context.Context, filePath string, req *models.OCRDocumentRequest) (*models.OCRDocumentResponse, error) {
	if filePath == "" {
		return nil, fmt.Errorf("ocr.documentFromFile: filePath must not be empty")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ocr.documentFromFile: read %s: %w", filePath, err)
	}
	var shell models.OCRDocumentRequest
	if req != nil {
		shell = *req
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	shell.Content = &b64
	return s.Document(ctx, &shell)
}
