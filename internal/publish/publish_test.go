package publish

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublish_UploadsFiles(t *testing.T) {
	var receivedFiles []string
	var receivedAuth string
	var receivedFiletype string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		receivedAuth = user + ":" + pass

		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if !strings.HasPrefix(mediaType, "multipart/") {
			t.Error("expected multipart request")
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if part.FormName() == "content" {
				receivedFiles = append(receivedFiles, part.FileName())
			}
			if part.FormName() == "filetype" {
				data, _ := io.ReadAll(part)
				receivedFiletype = string(data)
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a fake wheel file.
	dir := t.TempDir()
	wheelPath := filepath.Join(dir, "mypkg-0.1.0-py3-none-any.whl")
	os.WriteFile(wheelPath, []byte("fake wheel content"), 0644)

	err := Publish(Options{
		Files:      []string{wheelPath},
		Repository: server.URL,
		Token:      "pypi-test-token-123",
	})
	if err != nil {
		t.Fatal(err)
	}

	if receivedAuth != "__token__:pypi-test-token-123" {
		t.Errorf("auth = %q, want __token__:pypi-test-token-123", receivedAuth)
	}
	if len(receivedFiles) != 1 || receivedFiles[0] != "mypkg-0.1.0-py3-none-any.whl" {
		t.Errorf("received files = %v", receivedFiles)
	}
	if receivedFiletype != "bdist_wheel" {
		t.Errorf("filetype = %q, want bdist_wheel", receivedFiletype)
	}
}

func TestPublish_SdistFiletype(t *testing.T) {
	var receivedFiletype string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			if part.FormName() == "filetype" {
				data, _ := io.ReadAll(part)
				receivedFiletype = string(data)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dir := t.TempDir()
	sdistPath := filepath.Join(dir, "mypkg-0.1.0.tar.gz")
	os.WriteFile(sdistPath, []byte("fake sdist"), 0644)

	Publish(Options{
		Files:      []string{sdistPath},
		Repository: server.URL,
		Token:      "tok",
	})

	if receivedFiletype != "sdist" {
		t.Errorf("filetype = %q, want sdist", receivedFiletype)
	}
}

func TestPublish_NoToken(t *testing.T) {
	err := Publish(Options{
		Files:      []string{"/fake.whl"},
		Repository: "pypi",
		Token:      "",
	})
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no API token") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestPublish_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	dir := t.TempDir()
	wheelPath := filepath.Join(dir, "pkg-1.0.0-py3-none-any.whl")
	os.WriteFile(wheelPath, []byte("data"), 0644)

	err := Publish(Options{
		Files:      []string{wheelPath},
		Repository: server.URL,
		Token:      "bad-token",
	})
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("error = %q", err.Error())
	}
}
