package tools

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadManagerPutAndConsume(t *testing.T) {
	mgr, err := NewUploadManager("http://localhost:8560", t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("NewUploadManager() error = %v", err)
	}
	defer mgr.Close()

	reservation, err := mgr.RequestUpload("../companies.csv", "text/csv")
	if err != nil {
		t.Fatalf("RequestUpload() error = %v", err)
	}
	if reservation.FileName != "companies.csv" {
		t.Fatalf("FileName = %q, want companies.csv", reservation.FileName)
	}
	if !strings.Contains(reservation.CurlCommand, "/path/to/companies.csv") {
		t.Fatalf("CurlCommand = %q, want path hint", reservation.CurlCommand)
	}

	const body = "company,url\nAcme,https://example.com\n"
	req := httptest.NewRequest(http.MethodPut, reservation.UploadURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/csv")
	rr := httptest.NewRecorder()

	mgr.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rr.Code, rr.Body.String())
	}

	upload, err := mgr.ConsumeUpload(reservation.UploadID)
	if err != nil {
		t.Fatalf("ConsumeUpload() error = %v", err)
	}
	defer os.Remove(upload.Path)

	if upload.FileName != "companies.csv" {
		t.Fatalf("upload.FileName = %q, want companies.csv", upload.FileName)
	}
	if upload.ContentType != "text/csv" {
		t.Fatalf("upload.ContentType = %q, want text/csv", upload.ContentType)
	}
	if upload.Size != int64(len(body)) {
		t.Fatalf("upload.Size = %d, want %d", upload.Size, len(body))
	}

	got, err := os.ReadFile(upload.Path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != body {
		t.Fatalf("stored body = %q, want %q", string(got), body)
	}
}

func TestUploadManagerRejectsInvalidSignature(t *testing.T) {
	mgr, err := NewUploadManager("http://localhost:8560", t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("NewUploadManager() error = %v", err)
	}
	defer mgr.Close()

	reservation, err := mgr.RequestUpload("report.pdf", "")
	if err != nil {
		t.Fatalf("RequestUpload() error = %v", err)
	}

	uploadURL, err := url.Parse(reservation.UploadURL)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	q := uploadURL.Query()
	q.Set("sig", "bad")
	uploadURL.RawQuery = q.Encode()

	req := httptest.NewRequest(http.MethodPut, uploadURL.String(), strings.NewReader("data"))
	rr := httptest.NewRecorder()

	mgr.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("PUT status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUploadManagerRejectsTooLargeFile(t *testing.T) {
	mgr, err := NewUploadManager("http://localhost:8560", t.TempDir(), 3)
	if err != nil {
		t.Fatalf("NewUploadManager() error = %v", err)
	}
	defer mgr.Close()

	reservation, err := mgr.RequestUpload("oversized.txt", "text/plain")
	if err != nil {
		t.Fatalf("RequestUpload() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, reservation.UploadURL, strings.NewReader("1234"))
	rr := httptest.NewRecorder()

	mgr.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("PUT status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestNewUploadManagerSweepsStaleUploadFiles(t *testing.T) {
	dir := t.TempDir()
	staleUploadPath := filepath.Join(dir, uploadTempPrefix+"abc123-leftover")
	legacyStaleUploadPath := filepath.Join(dir, "0123456789abcdef0123456789abcdef-leftover")
	unrelatedPath := filepath.Join(dir, "keep-me")

	if err := os.WriteFile(staleUploadPath, []byte("old upload"), 0600); err != nil {
		t.Fatalf("WriteFile(staleUploadPath) error = %v", err)
	}
	if err := os.WriteFile(legacyStaleUploadPath, []byte("old legacy upload"), 0600); err != nil {
		t.Fatalf("WriteFile(legacyStaleUploadPath) error = %v", err)
	}
	if err := os.WriteFile(unrelatedPath, []byte("not ours"), 0600); err != nil {
		t.Fatalf("WriteFile(unrelatedPath) error = %v", err)
	}

	mgr, err := NewUploadManager("http://localhost:8560", dir, 1024)
	if err != nil {
		t.Fatalf("NewUploadManager() error = %v", err)
	}
	defer mgr.Close()

	if _, err := os.Stat(staleUploadPath); !os.IsNotExist(err) {
		t.Fatalf("stale upload file still exists, stat error = %v", err)
	}
	if _, err := os.Stat(legacyStaleUploadPath); !os.IsNotExist(err) {
		t.Fatalf("legacy stale upload file still exists, stat error = %v", err)
	}
	if _, err := os.Stat(unrelatedPath); err != nil {
		t.Fatalf("unrelated file was removed or inaccessible: %v", err)
	}
}
