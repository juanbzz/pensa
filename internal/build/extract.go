package build

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func validatePath(dest, name string) (string, error) {
	target := filepath.Join(dest, name)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid file path in archive: %s", name)
	}
	return target, nil
}

func ExtractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// sanitize path to prevent directory traversal and zip slip attacks
		target, err := validatePath(dest, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("create parent directory: %w", err)
			}
			if err := extractToFile(tarReader, target); err != nil {
				return err
			}
		default:
			// skip symlinks, hard links, and other special file types
			continue
		}
	}
	return nil
}

func ExtractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip file: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		path, err := validatePath(dest, f.Name)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
			// skip going into the directory since zip files list all entries with full paths
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}
		if err := extractZipFile(f, path); err != nil {
			return err
		}
	}
	return nil
}

func ExtractSdist(src, dest string) error {
	switch {
	case strings.HasSuffix(src, ".tar.gz") || strings.HasSuffix(src, ".tgz"):
		return ExtractTarGz(src, dest)
	case strings.HasSuffix(src, ".zip"):
		return ExtractZip(src, dest)
	default:
		return fmt.Errorf("unsupported sdist format: %s", src)
	}
}

func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry: %w", err)
	}
	defer rc.Close()

	return extractToFile(rc, dest)
}

func extractToFile(r io.Reader, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}
	return nil
}
