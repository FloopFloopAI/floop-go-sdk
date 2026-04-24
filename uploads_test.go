package floop

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newUploadTestClient stands up two servers: one for the FloopFloop
// presign + main API ("floop"), and one that pretends to be S3 for the
// direct PUT. The presign response rewrites uploadUrl to point at the
// S3 test server so the full two-hop flow can be tested offline.
func newUploadTestClient(
	t *testing.T,
	presignHandler http.HandlerFunc,
	s3Handler http.HandlerFunc,
) *Client {
	t.Helper()
	s3 := httptest.NewServer(s3Handler)
	t.Cleanup(s3.Close)

	floop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rewrite any uploadUrl field the handler writes so tests can
		// compose {"data":{"uploadUrl":"%S3URL%",...}}.
		rec := httptest.NewRecorder()
		presignHandler(rec, r)
		body := strings.ReplaceAll(rec.Body.String(), "%S3URL%", s3.URL)
		for k, v := range rec.Header() {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		if rec.Code != 0 {
			w.WriteHeader(rec.Code)
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(floop.Close)

	c, err := NewClient("flp_test", WithBaseURL(floop.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestUploads_HappyPath_Bytes(t *testing.T) {
	var seenPresign, seenS3 string
	var s3Body []byte
	var s3ContentType string
	c := newUploadTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			seenPresign = r.URL.Path
			w.Write([]byte(`{"data":{"uploadUrl":"%S3URL%/put","key":"uploads/u_1/cat.png","fileId":"f_1"}}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			seenS3 = r.URL.Path
			s3ContentType = r.Header.Get("Content-Type")
			s3Body, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		},
	)

	payload := []byte("fake-png-bytes")
	out, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "cat.png",
		Bytes:    payload,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if seenPresign != "/api/v1/uploads" {
		t.Errorf("presign path: %s", seenPresign)
	}
	if seenS3 != "/put" {
		t.Errorf("S3 path: %s", seenS3)
	}
	if s3ContentType != "image/png" {
		t.Errorf("S3 content-type: %s", s3ContentType)
	}
	if string(s3Body) != string(payload) {
		t.Errorf("S3 body: got %q, want %q", string(s3Body), string(payload))
	}
	if out.Key != "uploads/u_1/cat.png" || out.FileName != "cat.png" || out.FileType != "image/png" || out.FileSize != int64(len(payload)) {
		t.Errorf("result: %+v", out)
	}
}

func TestUploads_StreamingWithSize(t *testing.T) {
	var s3Body []byte
	c := newUploadTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":{"uploadUrl":"%S3URL%/p","key":"uploads/k","fileId":"f"}}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			s3Body, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		},
	)
	src := "streaming-bytes-here"
	out, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "notes.txt",
		File:     strings.NewReader(src),
		Size:     int64(len(src)),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if string(s3Body) != src {
		t.Errorf("stream body: got %q", string(s3Body))
	}
	if out.FileSize != int64(len(src)) {
		t.Errorf("size: %d", out.FileSize)
	}
}

func TestUploads_ExplicitFileTypeOverridesGuess(t *testing.T) {
	var seenContentType string
	c := newUploadTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":{"uploadUrl":"%S3URL%/p","key":"k","fileId":"f"}}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			seenContentType = r.Header.Get("Content-Type")
			w.WriteHeader(200)
		},
	)
	_, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "dataset.csv", // ext → text/csv
		Bytes:    []byte("a,b\n1,2\n"),
		FileType: "text/plain", // explicit override, on allowlist
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenContentType != "text/plain" {
		t.Errorf("expected override to text/plain, got %s", seenContentType)
	}
}

func TestUploads_UnknownMimeRejected(t *testing.T) {
	c, _ := NewClient("flp_test", WithBaseURL("http://unused.example"))
	_, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "archive.tar.gz", // not on allowlist
		Bytes:    []byte("whatever"),
	})
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestUploads_ExceedsSizeLimit(t *testing.T) {
	c, _ := NewClient("flp_test", WithBaseURL("http://unused.example"))
	big := make([]byte, MaxUploadBytes+1)
	_, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "big.png",
		Bytes:    big,
	})
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
	if !strings.Contains(fe.Message, "upload limit") {
		t.Errorf("error should mention the limit: %q", fe.Message)
	}
}

func TestUploads_MissingBodyRejected(t *testing.T) {
	c, _ := NewClient("flp_test", WithBaseURL("http://unused.example"))
	_, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "notes.txt",
	})
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestUploads_BothBodiesRejected(t *testing.T) {
	c, _ := NewClient("flp_test", WithBaseURL("http://unused.example"))
	_, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "notes.txt",
		Bytes:    []byte("a"),
		File:     strings.NewReader("b"),
		Size:     1,
	})
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %v", err)
	}
}

func TestUploads_S3ErrorSurfaces(t *testing.T) {
	c := newUploadTestClient(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":{"uploadUrl":"%S3URL%/p","key":"k","fileId":"f"}}`))
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(403)
			w.Write([]byte(`<Error><Code>AccessDenied</Code></Error>`))
		},
	)
	_, err := c.Uploads.Create(context.Background(), CreateUploadInput{
		FileName: "cat.png",
		Bytes:    []byte("x"),
	})
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if fe.Status != 403 {
		t.Errorf("status: %d", fe.Status)
	}
	if !strings.Contains(fe.Message, "AccessDenied") {
		t.Errorf("message should include raw S3 body: %q", fe.Message)
	}
}

func TestGuessMimeType(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"cat.PNG", "image/png"},
		{"notes.txt", "text/plain"},
		{"resume.DOCX", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"archive.tar.gz", ""},
		{"noext", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := GuessMimeType(tc.in); got != tc.want {
			t.Errorf("GuessMimeType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
