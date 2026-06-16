package workspacebundle

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/agentsafe/agentsafe/internal/config"
)

const (
	BundleVersion = 1
	manifestName  = "bundle.json"
	configName    = "config.yaml"
	securityName  = "agentsafe.yaml"
	templatesDir  = "worktree-templates"
)

type Manifest struct {
	Version       int    `json:"version"`
	ExportedAt    string `json:"exportedAt"`
	WorkspaceName string `json:"workspaceName"`
}

func Export(root, dst string) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	manifest, _ := json.MarshalIndent(Manifest{
		Version:       BundleVersion,
		ExportedAt:    time.Now().Format(time.RFC3339),
		WorkspaceName: cfg.Workspace.Name,
	}, "", "  ")
	if err := addBytes(zw, manifestName, manifest); err != nil {
		return err
	}
	cfgBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := addBytes(zw, configName, cfgBytes); err != nil {
		return err
	}
	if err := addFileIfExists(zw, filepath.Join(root, securityName), securityName); err != nil {
		return err
	}
	return addDirIfExists(zw, filepath.Join(root, config.DirName, templatesDir), templatesDir)
}

func Import(zipPath, target string) (config.Config, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return config.Config{}, err
	}
	if err := ensureEmptyNonWorkspace(absTarget); err != nil {
		return config.Config{}, err
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return config.Config{}, err
	}
	defer zr.Close()

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		clean := filepath.ToSlash(filepath.Clean(f.Name))
		clean = strings.TrimPrefix(clean, "./")
		if clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
			return config.Config{}, fmt.Errorf("invalid bundle path %q", f.Name)
		}
		files[clean] = f
	}
	cfgFile := files[configName]
	if cfgFile == nil {
		return config.Config{}, fmt.Errorf("bundle missing %s", configName)
	}
	cfgBytes, err := readZipFile(cfgFile)
	if err != nil {
		return config.Config{}, err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		return config.Config{}, err
	}
	cfg.Workspace.Name = filepath.Base(absTarget)
	cfg.Workspace.Root = absTarget

	if err := createWorkspaceDirs(absTarget); err != nil {
		return config.Config{}, err
	}
	if err := config.Save(absTarget, cfg); err != nil {
		return config.Config{}, err
	}
	if f := files[securityName]; f != nil {
		if err := extractFile(f, filepath.Join(absTarget, securityName)); err != nil {
			return config.Config{}, err
		}
	}
	for name, f := range files {
		if name == templatesDir || !strings.HasPrefix(name, templatesDir+"/") {
			continue
		}
		rel := strings.TrimPrefix(name, templatesDir+"/")
		dst := filepath.Join(absTarget, config.DirName, templatesDir, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return config.Config{}, err
			}
			continue
		}
		if err := extractFile(f, dst); err != nil {
			return config.Config{}, err
		}
	}
	return cfg, nil
}

func ensureEmptyNonWorkspace(target string) error {
	if _, err := os.Stat(filepath.Join(target, config.DirName, config.ConfigFileName)); err == nil {
		return fmt.Errorf("target is already an agentsafe workspace: %s", target)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("target directory must be empty: %s", target)
	}
	return nil
}

func createWorkspaceDirs(root string) error {
	for _, d := range []string{
		config.DirName,
		"main",
		"feature",
		"agent",
		filepath.Join(config.DirName, "features"),
		filepath.Join(config.DirName, "sessions"),
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func addBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func addFileIfExists(zw *zip.Writer, src, name string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func addDirIfExists(zw *zip.Writer, src, zipRoot string) error {
	if st, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	} else if !st.IsDir() {
		return nil
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		return addFileIfExists(zw, path, filepath.ToSlash(filepath.Join(zipRoot, rel)))
	})
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func extractFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.FileInfo().Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
