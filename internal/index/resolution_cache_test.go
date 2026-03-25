package index

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

// CachedVersionDeps is a lightweight struct for the resolution cache.
// Only stores what the resolver needs — no file lists, no URLs.
type CachedVersionDeps struct {
	Version      string
	Dependencies []string // PEP 508 dependency strings
}

type CachedPackage struct {
	Name     string
	Versions []string // available version strings
	Deps     map[string]CachedVersionDeps // version → deps
}

func makeTestData(n int) []CachedPackage {
	var pkgs []CachedPackage
	for i := 0; i < n; i++ {
		pkg := CachedPackage{
			Name:     fmt.Sprintf("package-%d", i),
			Versions: make([]string, 20),
			Deps:     make(map[string]CachedVersionDeps),
		}
		for j := 0; j < 20; j++ {
			ver := fmt.Sprintf("%d.%d.%d", j/10, j%10, 0)
			pkg.Versions[j] = ver
			pkg.Deps[ver] = CachedVersionDeps{
				Version: ver,
				Dependencies: []string{
					fmt.Sprintf("dep-a>=%d.0", j),
					fmt.Sprintf("dep-b>=%d.0,<%d.0", j, j+1),
					"dep-c>=1.0",
				},
			}
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func BenchmarkJSON_Write(b *testing.B) {
	pkgs := makeTestData(130)
	dir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pkg := range pkgs {
			data, _ := json.Marshal(pkg)
			os.WriteFile(filepath.Join(dir, pkg.Name+".json"), data, 0644)
		}
	}
}

func BenchmarkJSON_Read(b *testing.B) {
	pkgs := makeTestData(130)
	dir := b.TempDir()
	for _, pkg := range pkgs {
		data, _ := json.Marshal(pkg)
		os.WriteFile(filepath.Join(dir, pkg.Name+".json"), data, 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pkg := range pkgs {
			data, _ := os.ReadFile(filepath.Join(dir, pkg.Name+".json"))
			var p CachedPackage
			json.Unmarshal(data, &p)
		}
	}
}

func BenchmarkGob_Write(b *testing.B) {
	pkgs := makeTestData(130)
	dir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pkg := range pkgs {
			var buf bytes.Buffer
			gob.NewEncoder(&buf).Encode(pkg)
			os.WriteFile(filepath.Join(dir, pkg.Name+".gob"), buf.Bytes(), 0644)
		}
	}
}

func BenchmarkGob_Read(b *testing.B) {
	pkgs := makeTestData(130)
	dir := b.TempDir()
	for _, pkg := range pkgs {
		var buf bytes.Buffer
		gob.NewEncoder(&buf).Encode(pkg)
		os.WriteFile(filepath.Join(dir, pkg.Name+".gob"), buf.Bytes(), 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pkg := range pkgs {
			data, _ := os.ReadFile(filepath.Join(dir, pkg.Name+".gob"))
			var p CachedPackage
			gob.NewDecoder(bytes.NewReader(data)).Decode(&p)
		}
	}
}

func BenchmarkMsgpack_Write(b *testing.B) {
	pkgs := makeTestData(130)
	dir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pkg := range pkgs {
			data, _ := msgpack.Marshal(pkg)
			os.WriteFile(filepath.Join(dir, pkg.Name+".msgpack"), data, 0644)
		}
	}
}

func BenchmarkMsgpack_Read(b *testing.B) {
	pkgs := makeTestData(130)
	dir := b.TempDir()
	for _, pkg := range pkgs {
		data, _ := msgpack.Marshal(pkg)
		os.WriteFile(filepath.Join(dir, pkg.Name+".msgpack"), data, 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pkg := range pkgs {
			data, _ := os.ReadFile(filepath.Join(dir, pkg.Name+".msgpack"))
			var p CachedPackage
			msgpack.Unmarshal(data, &p)
		}
	}
}

func BenchmarkFileSize(b *testing.B) {
	pkg := makeTestData(1)[0]

	jsonData, _ := json.Marshal(pkg)
	var gobBuf bytes.Buffer
	gob.NewEncoder(&gobBuf).Encode(pkg)
	msgpackData, _ := msgpack.Marshal(pkg)

	b.Logf("JSON: %d bytes, Gob: %d bytes, MsgPack: %d bytes",
		len(jsonData), gobBuf.Len(), len(msgpackData))
}
