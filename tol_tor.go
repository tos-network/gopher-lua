package lua

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var torZipMagic = [4]byte{'P', 'K', 0x03, 0x04}

const torManifestPath = "manifest.json"

var torDeterministicModTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// TORArtifact is a decoded .tor archive payload.
type TORArtifact struct {
	ManifestJSON []byte
	Files        map[string][]byte // excludes manifest.json
}

// TORCompileOptions configures one-shot .tol -> .tor compilation.
type TORCompileOptions struct {
	PackageName    string
	PackageVersion string
	TOCPath        string
	TOIPath        string
	IncludeSource  bool
	SourcePath     string
}

// IsTOR reports whether input starts with local-file ZIP magic.
func IsTOR(data []byte) bool {
	if len(data) < len(torZipMagic) {
		return false
	}
	for i := range torZipMagic {
		if data[i] != torZipMagic[i] {
			return false
		}
	}
	return true
}

// TORPackageHash computes keccak256 hash of a .tor archive.
func TORPackageHash(data []byte) string {
	return keccak256Hex(data)
}

// CompileTOLToTOR compiles source into a minimal deterministic .tor package.
func CompileTOLToTOR(source []byte, name string, opts *TORCompileOptions) ([]byte, error) {
	mod, err := ParseTOLModule(source, name)
	if err != nil {
		return nil, err
	}
	if mod == nil || mod.Contract == nil || strings.TrimSpace(mod.Contract.Name) == "" {
		return nil, fmt.Errorf("tor compile requires contract declaration")
	}
	contractName := strings.TrimSpace(mod.Contract.Name)

	toc, err := CompileTOLToTOC(source, name)
	if err != nil {
		return nil, err
	}
	toi, err := BuildTOIFromModule(mod)
	if err != nil {
		return nil, err
	}

	pkgName := strings.ToLower(contractName)
	pkgVersion := "0.1.0"
	tocPath := fmt.Sprintf("bytecode/%s.toc", contractName)
	toiPath := fmt.Sprintf("interfaces/I%s.toi", contractName)
	includeSource := false
	sourcePath := ""

	if opts != nil {
		if strings.TrimSpace(opts.PackageName) != "" {
			pkgName = strings.TrimSpace(opts.PackageName)
		}
		if strings.TrimSpace(opts.PackageVersion) != "" {
			pkgVersion = strings.TrimSpace(opts.PackageVersion)
		}
		if strings.TrimSpace(opts.TOCPath) != "" {
			tocPath = strings.TrimSpace(opts.TOCPath)
		}
		if strings.TrimSpace(opts.TOIPath) != "" {
			toiPath = strings.TrimSpace(opts.TOIPath)
		}
		includeSource = opts.IncludeSource
		sourcePath = strings.TrimSpace(opts.SourcePath)
	}
	if includeSource {
		if sourcePath == "" {
			base := filepath.Base(name)
			if base == "." || base == "/" || base == string(filepath.Separator) || base == "" {
				base = contractName + ".tol"
			}
			sourcePath = "sources/" + base
		}
	}

	manifest := struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		TOLVersion string `json:"tol_version"`
		Compiler   string `json:"compiler"`
		Contracts  []struct {
			Name string `json:"name"`
			TOC  string `json:"toc"`
			TOI  string `json:"toi"`
		} `json:"contracts"`
	}{
		Name:       pkgName,
		Version:    pkgVersion,
		TOLVersion: strings.TrimSpace(mod.Version),
		Compiler:   "tolang/" + PackageVersion,
		Contracts: []struct {
			Name string `json:"name"`
			TOC  string `json:"toc"`
			TOI  string `json:"toi"`
		}{
			{Name: contractName, TOC: tocPath, TOI: toiPath},
		},
	}
	if manifest.TOLVersion == "" {
		manifest.TOLVersion = "0.2"
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{
		tocPath: toc,
		toiPath: toi,
	}
	if includeSource {
		files[sourcePath] = source
	}
	return EncodeTOR(manifestJSON, files)
}

// EncodeTOR serializes manifest + files into deterministic .tor bytes.
func EncodeTOR(manifestJSON []byte, files map[string][]byte) ([]byte, error) {
	cleanFiles := map[string][]byte{}
	for name, body := range files {
		clean, err := normalizeTORPath(name)
		if err != nil {
			return nil, err
		}
		if clean == torManifestPath {
			return nil, fmt.Errorf("tor files must not override %q", torManifestPath)
		}
		b := make([]byte, len(body))
		copy(b, body)
		cleanFiles[clean] = b
	}
	if err := validateTORManifest(manifestJSON, cleanFiles, true); err != nil {
		return nil, err
	}

	var names []string
	for name := range cleanFiles {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeTORZipEntry(zw, torManifestPath, manifestJSON); err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := writeTORZipEntry(zw, name, cleanFiles[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeTOR deserializes .tor bytes and validates manifest/files.
func DecodeTOR(data []byte) (*TORArtifact, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid tor zip: %w", err)
	}

	seen := map[string]struct{}{}
	var manifest []byte
	files := map[string][]byte{}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, err := normalizeTORPath(f.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate tor entry: %s", name)
		}
		seen[name] = struct{}{}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if name == torManifestPath {
			manifest = body
			continue
		}
		files[name] = body
	}

	if len(manifest) == 0 {
		return nil, fmt.Errorf("tor manifest.json not found")
	}
	if err := validateTORManifest(manifest, files, true); err != nil {
		return nil, err
	}
	for name, body := range files {
		if strings.HasSuffix(strings.ToLower(name), ".toc") {
			if _, err := DecodeTOC(body); err != nil {
				return nil, fmt.Errorf("invalid .toc entry %q: %w", name, err)
			}
		}
	}
	return &TORArtifact{
		ManifestJSON: manifest,
		Files:        files,
	}, nil
}

func validateTORManifest(manifestJSON []byte, files map[string][]byte, verifyRefs bool) error {
	if !json.Valid(manifestJSON) {
		return fmt.Errorf("tor manifest is not valid json")
	}
	var m struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Contracts []struct {
			Name string `json:"name"`
			TOC  string `json:"toc"`
			TOI  string `json:"toi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return fmt.Errorf("tor manifest decode error: %w", err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("tor manifest requires non-empty 'name'")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("tor manifest requires non-empty 'version'")
	}
	if verifyRefs {
		for _, c := range m.Contracts {
			if p := strings.TrimSpace(c.TOC); p != "" {
				np, err := normalizeTORPath(p)
				if err != nil {
					return fmt.Errorf("tor manifest contract %q has invalid toc path %q: %w", c.Name, p, err)
				}
				if _, ok := files[np]; !ok {
					return fmt.Errorf("tor manifest contract %q references missing toc file %q", c.Name, np)
				}
			}
			if p := strings.TrimSpace(c.TOI); p != "" {
				np, err := normalizeTORPath(p)
				if err != nil {
					return fmt.Errorf("tor manifest contract %q has invalid toi path %q: %w", c.Name, p, err)
				}
				if _, ok := files[np]; !ok {
					return fmt.Errorf("tor manifest contract %q references missing toi file %q", c.Name, np)
				}
			}
		}
	}
	return nil
}

func writeTORZipEntry(zw *zip.Writer, name string, body []byte) error {
	hdr := &zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	}
	hdr.SetModTime(torDeterministicModTime)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	_, err = w.Write(body)
	return err
}

func normalizeTORPath(p string) (string, error) {
	name := strings.TrimSpace(p)
	if name == "" {
		return "", fmt.Errorf("tor entry path is empty")
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return "", fmt.Errorf("tor entry path must be relative: %q", p)
	}
	name = strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("tor entry path escapes archive root: %q", p)
	}
	if strings.Contains(clean, "/../") {
		return "", fmt.Errorf("tor entry path escapes archive root: %q", p)
	}
	return clean, nil
}
