package floop

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// MaxUploadBytes is the per-file ceiling enforced by the backend.
// Matches the Node/Python SDKs and the CLI.
const MaxUploadBytes int64 = 5 * 1024 * 1024

// extToMime is the allowlist of file extensions the backend accepts.
// Keep this in sync with the Node SDK's uploads.ts — the backend
// rejects any fileType not on this list with VALIDATION_ERROR.
var extToMime = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
	".ico":  "image/x-icon",
	".pdf":  "application/pdf",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
}

// GuessMimeType returns the MIME type implied by a filename's extension,
// or the empty string if the extension isn't on the backend allowlist.
// Exported so callers can inspect / override before calling Create.
func GuessMimeType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	return extToMime[ext]
}

func isAllowedMime(mime string) bool {
	for _, v := range extToMime {
		if v == mime {
			return true
		}
	}
	return false
}

// UploadedAttachment is what Uploads.Create returns — the shape the
// backend expects in Projects.Refine's Attachments slice.
type UploadedAttachment struct {
	Key      string `json:"key"`
	FileName string `json:"fileName"`
	FileType string `json:"fileType"`
	FileSize int64  `json:"fileSize"`
}

// CreateUploadInput describes a single file to upload.
//
// One of File OR Bytes must be set:
//
//   - File + Size: stream an io.Reader of the given length. Preferred for
//     large files (e.g. wrap an *os.File after a stat to avoid double-
//     reading into memory).
//   - Bytes: pass the raw bytes. Convenient when you already have the
//     payload buffered.
//
// FileType is optional — if empty, it's guessed from FileName's
// extension. If no guess is possible or the provided type isn't on the
// backend allowlist, Create returns a *floop.Error with Code
// "VALIDATION_ERROR".
type CreateUploadInput struct {
	FileName string
	FileType string

	// Streaming path: supply File + Size.
	File io.Reader
	Size int64

	// Buffered path: supply Bytes (Size is derived).
	Bytes []byte
}

type uploadPresignRequest struct {
	FileName string `json:"fileName"`
	FileType string `json:"fileType"`
	FileSize int64  `json:"fileSize"`
}

type uploadPresignResponse struct {
	UploadURL string `json:"uploadUrl"`
	Key       string `json:"key"`
	FileID    string `json:"fileId"`
}

// Uploads is the resource namespace for attachment uploads. The backend
// presigns an S3 URL; Create then PUTs the bytes directly to S3 and
// returns a reference that can be attached to a subsequent
// Projects.Refine call.
type Uploads struct {
	client *Client
}

// Create presigns an upload slot on the backend, PUTs the file to S3,
// and returns an UploadedAttachment you can pass to Projects.Refine.
//
// Input validation mirrors the Node/Python SDKs: rejects files larger
// than 5 MB, rejects MIME types not on the backend allowlist (derived
// from the file extension when FileType is not explicit).
func (u *Uploads) Create(ctx context.Context, input CreateUploadInput) (*UploadedAttachment, error) {
	// Resolve body + size up front so we can validate once.
	var body io.Reader
	var size int64
	switch {
	case len(input.Bytes) > 0 && input.File != nil:
		return nil, &Error{
			Code:    "VALIDATION_ERROR",
			Message: "uploads: exactly one of File or Bytes must be set",
		}
	case len(input.Bytes) > 0:
		body = bytes.NewReader(input.Bytes)
		size = int64(len(input.Bytes))
	case input.File != nil:
		if input.Size <= 0 {
			return nil, &Error{
				Code:    "VALIDATION_ERROR",
				Message: "uploads: Size is required when File is a stream",
			}
		}
		body = input.File
		size = input.Size
	default:
		return nil, &Error{
			Code:    "VALIDATION_ERROR",
			Message: "uploads: one of File or Bytes must be set",
		}
	}

	if input.FileName == "" {
		return nil, &Error{
			Code:    "VALIDATION_ERROR",
			Message: "uploads: FileName is required",
		}
	}

	fileType := input.FileType
	if fileType == "" {
		fileType = GuessMimeType(input.FileName)
	}
	if fileType == "" || !isAllowedMime(fileType) {
		return nil, &Error{
			Code: "VALIDATION_ERROR",
			Message: fmt.Sprintf(
				"uploads: unsupported file type for %s. Allowed: png, jpg, gif, svg, webp, ico, pdf, txt, csv, doc, docx.",
				input.FileName,
			),
		}
	}

	if size > MaxUploadBytes {
		return nil, &Error{
			Code: "VALIDATION_ERROR",
			Message: fmt.Sprintf(
				"uploads: %s is %.1f MB — the upload limit is %d MB.",
				input.FileName,
				float64(size)/(1024*1024),
				MaxUploadBytes/(1024*1024),
			),
		}
	}

	// 1. Presign
	var presign uploadPresignResponse
	err := u.client.request(ctx, "POST", "/api/v1/uploads", uploadPresignRequest{
		FileName: input.FileName,
		FileType: fileType,
		FileSize: size,
	}, &presign)
	if err != nil {
		return nil, err
	}

	// 2. Direct PUT to S3. We do NOT send bearer auth here — the
	// presigned URL already carries its signature. Using the same
	// httpClient keeps proxies / timeouts consistent.
	req, reqErr := http.NewRequestWithContext(ctx, "PUT", presign.UploadURL, body)
	if reqErr != nil {
		return nil, &Error{
			Code:    "UNKNOWN",
			Message: "uploads: failed to build S3 PUT request: " + reqErr.Error(),
		}
	}
	req.Header.Set("Content-Type", fileType)
	req.ContentLength = size

	resp, putErr := u.client.httpClient.Do(req)
	if putErr != nil {
		return nil, &Error{
			Code:    "NETWORK_ERROR",
			Message: "uploads: S3 PUT failed — " + putErr.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// S3 returns XML. Read a prefix for the error message — don't
		// try to parse it; just include raw bytes truncated.
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &Error{
			Code:    "UNKNOWN",
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("uploads: S3 rejected PUT (%d): %s", resp.StatusCode, string(buf)),
		}
	}

	return &UploadedAttachment{
		Key:      presign.Key,
		FileName: input.FileName,
		FileType: fileType,
		FileSize: size,
	}, nil
}
