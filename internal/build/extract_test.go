package build

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"github.com/matryer/is"
	"os"
	"path/filepath"
	"testing"
)

type testEntry struct {
	Name     string
	Content  string
	TypeFlag byte
	Linkname string
}

func createTarGz(t *testing.T, files []testEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, entry := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     entry.Name,
			Size:     int64(len(entry.Content)),
			Mode:     0644,
			Typeflag: entry.TypeFlag,
			Linkname: entry.Linkname,
		}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}

		if _, err := tw.Write([]byte(entry.Content)); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}

	tw.Close()
	gw.Close()

	return buf.Bytes()
}

func createZip(t *testing.T, files []testEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, entry := range files {
		f, err := zw.Create(entry.Name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}

		if _, err := f.Write([]byte(entry.Content)); err != nil {
			t.Fatalf("write zip content: %v", err)
		}
	}
	zw.Close()

	return buf.Bytes()
}

func TestExtractTarGz(t *testing.T) {
	assert := is.New(t)

	tarFile := createTarGz(t, []testEntry{
		{Name: "pkg/file.txt", Content: "hello", TypeFlag: tar.TypeReg},
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	tmpFile := filepath.Join(srcDir, "test.tar.gz")
	if err := os.WriteFile(tmpFile, tarFile, 0644); err != nil {
		t.Fatalf("write tar.gz to temp file: %v", err)
	}

	err := ExtractTarGz(tmpFile, destDir)
	assert.NoErr(err)

	extractedFile := filepath.Join(destDir, "pkg/file.txt")
	data, err := os.ReadFile(extractedFile)
	assert.NoErr(err)
	assert.Equal(string(data), "hello")
}

func TestExtractTarGz_ZipSlip(t *testing.T) {
	assert := is.New(t)

	tarFile := createTarGz(t, []testEntry{
		{Name: "../../etc/passwd", Content: "evil", TypeFlag: tar.TypeReg},
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	tmpFile := filepath.Join(srcDir, "test.tar.gz")

	err := os.WriteFile(tmpFile, tarFile, 0644)
	assert.NoErr(err)

	err = ExtractTarGz(tmpFile, destDir)
	assert.True(err != nil) // extraction should be rejected
}

func TestExtractTarGz_SkipsSymLinks(t *testing.T) {
	assert := is.New(t)

	tarFile := createTarGz(t, []testEntry{
		{Name: "file.txt", Content: "hello", TypeFlag: tar.TypeReg},
		{Name: "link.txt", Linkname: "file.txt", TypeFlag: tar.TypeSymlink},
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	tmpFile := filepath.Join(srcDir, "test.tar.gz")

	err := os.WriteFile(tmpFile, tarFile, 0644)
	assert.NoErr(err)

	err = ExtractTarGz(tmpFile, destDir)
	assert.NoErr(err)

	// regular file should be extracted
	file := filepath.Join(destDir, "file.txt")
	data, err := os.ReadFile(file)
	assert.NoErr(err)
	assert.Equal(string(data), "hello")

	// symlink should be skipped
	link := filepath.Join(destDir, "link.txt")
	_, err = os.Lstat(link)
	assert.True(os.IsNotExist(err))
}

func TestExtractZip(t *testing.T) {
	assert := is.New(t)

	zipFile := createZip(t, []testEntry{
		{Name: "pkg/file.txt", Content: "hello"},
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	tmpFile := filepath.Join(srcDir, "test.zip")
	err := os.WriteFile(tmpFile, zipFile, 0644)
	assert.NoErr(err)

	err = ExtractZip(tmpFile, destDir)
	assert.NoErr(err)

	extractedFile := filepath.Join(destDir, "pkg/file.txt")
	data, err := os.ReadFile(extractedFile)
	assert.NoErr(err)
	assert.Equal(string(data), "hello")
}

func TestExtractZip_ZipSlip(t *testing.T) {
	assert := is.New(t)

	zipFile := createZip(t, []testEntry{
		{Name: "../../etc/passwd", Content: "evil"},
	})

	srcDir := t.TempDir()
	destDir := t.TempDir()
	tmpFile := filepath.Join(srcDir, "test.zip")
	err := os.WriteFile(tmpFile, zipFile, 0644)
	assert.NoErr(err)

	err = ExtractZip(tmpFile, destDir)
	assert.True(err != nil) // extraction should be rejected
}
