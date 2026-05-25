package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Version      int               `yaml:"version"`
	Name         string            `yaml:"name"`
	Repo         string            `yaml:"repo"`
	Environments string            `yaml:"environments"`
	Docker       Docker            `yaml:"docker"`
	VMScripts    map[string]Script `yaml:"vm_scripts"`
	EnvScripts   map[string]Script `yaml:"env_scripts"`
	Files        map[string]string `yaml:"files"`
	BaseDir      string            `yaml:"-"`
}

type Docker struct {
	ComposeFile    string `yaml:"compose_file"`
	ProjectPattern string `yaml:"project_pattern"`
}

type Script struct {
	Path    string   `yaml:"path"`
	Args    []string `yaml:"args"`
	Timeout string   `yaml:"timeout"`
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var mf Manifest
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return Manifest{}, err
	}
	if err := mf.Validate("."); err != nil {
		return Manifest{}, err
	}
	mf.BaseDir = "."
	return mf, nil
}

func (mf Manifest) AgentID() string {
	return slug(mf.Name)
}

func (mf Manifest) Validate(base string) error {
	if mf.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d", mf.Version)
	}
	if strings.TrimSpace(mf.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(mf.Environments) == "" {
		return fmt.Errorf("environments is required")
	}
	for name, script := range mf.VMScripts {
		if err := safeRelativePath(base, script.Path); err != nil {
			return fmt.Errorf("vm script %q: %w", name, err)
		}
	}
	for name, script := range mf.EnvScripts {
		if err := safeRelativePath(base, script.Path); err != nil {
			return fmt.Errorf("env script %q: %w", name, err)
		}
	}
	for name, file := range mf.Files {
		if err := safeRelativePath(base, file); err != nil {
			return fmt.Errorf("file %q: %w", name, err)
		}
	}
	return nil
}

func safeRelativePath(base, value string) error {
	if value == "" {
		return fmt.Errorf("path is required")
	}
	if filepath.IsAbs(value) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(value)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("path traversal is not allowed")
	}
	_, _ = filepath.Abs(filepath.Join(base, clean))
	return nil
}

func (mf Manifest) Resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	base := mf.BaseDir
	if base == "" {
		base = "."
	}
	return filepath.Join(base, filepath.Clean(path))
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", ".", "-")
	return replacer.Replace(value)
}
