package publish

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Known repository URLs.
var repositories = map[string]string{
	"pypi":     "https://upload.pypi.org/legacy/",
	"testpypi": "https://test.pypi.org/legacy/",
}

// Options controls publishing behavior.
type Options struct {
	Files      []string // paths to .whl and .tar.gz files
	Repository string   // "pypi", "testpypi", or a full URL
	Token      string   // PyPI API token
}

// Publish uploads distribution files to a Python package index.
func Publish(opts Options) error {
	if opts.Token == "" {
		return fmt.Errorf("no API token provided. Use --token or set PENSA_PYPI_TOKEN")
	}

	repoURL := opts.Repository
	if url, ok := repositories[strings.ToLower(repoURL)]; ok {
		repoURL = url
	}

	for _, filePath := range opts.Files {
		if err := uploadFile(repoURL, opts.Token, filePath); err != nil {
			return fmt.Errorf("upload %s: %w", filepath.Base(filePath), err)
		}
	}

	return nil
}

// uploadFile uploads a single distribution file to the repository.
func uploadFile(repoURL, token, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	filename := filepath.Base(filePath)

	// Determine filetype from extension.
	filetype := "sdist"
	if strings.HasSuffix(filename, ".whl") {
		filetype = "bdist_wheel"
	}

	// Build multipart form.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writer.WriteField(":action", "file_upload")
	writer.WriteField("protocol_version", "1")
	writer.WriteField("filetype", filetype)

	part, err := writer.CreateFormFile("content", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	writer.Close()

	req, err := http.NewRequest("POST", repoURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBasicAuth("__token__", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusForbidden:
		return fmt.Errorf("authentication failed (403). Check your API token")
	case http.StatusBadRequest:
		if strings.Contains(string(respBody), "already exists") {
			return fmt.Errorf("%s already exists on the repository", filename)
		}
		return fmt.Errorf("bad request (400): %s", string(respBody))
	default:
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(respBody))
	}
}
