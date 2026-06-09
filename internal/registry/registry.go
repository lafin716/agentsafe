// Package registry persists the set of agentsafe workspaces known to the
// desktop app, plus which one is active, in a single global JSON file under the
// user's config directory. Each workspace on disk is just a folder containing
// .agentsafe/; this registry is what lets the GUI list and switch between them.
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Entry is a single registered workspace.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"` // absolute workspace root
}

// Registry is the persisted set of workspaces and the active selection.
type Registry struct {
	Workspaces []Entry `json:"workspaces"`
	Active     string  `json:"active"` // path of the active workspace ("" when none)
}

// Path returns the location of the registry file, e.g.
// ~/Library/Application Support/agentsafe/workspaces.json on macOS.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agentsafe", "workspaces.json"), nil
}

// Load reads the registry, returning an empty one when the file does not exist.
func Load() (Registry, error) {
	p, err := Path()
	if err != nil {
		return Registry{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Workspaces: []Entry{}}, nil
		}
		return Registry{}, err
	}
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return Registry{}, err
	}
	if r.Workspaces == nil {
		r.Workspaces = []Entry{}
	}
	return r, nil
}

// Save writes the registry, creating the parent directory as needed.
func Save(r Registry) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0644)
}

// Add registers a workspace (deduped by path), updates its name, and marks it
// active.
func Add(name, path string) error {
	r, err := Load()
	if err != nil {
		return err
	}
	found := false
	for i := range r.Workspaces {
		if r.Workspaces[i].Path == path {
			r.Workspaces[i].Name = name
			found = true
			break
		}
	}
	if !found {
		r.Workspaces = append(r.Workspaces, Entry{Name: name, Path: path})
	}
	r.Active = path
	return Save(r)
}

// Remove unregisters a workspace. When it was the active one, the active
// selection is cleared.
func Remove(path string) error {
	r, err := Load()
	if err != nil {
		return err
	}
	out := r.Workspaces[:0]
	for _, e := range r.Workspaces {
		if e.Path != path {
			out = append(out, e)
		}
	}
	r.Workspaces = out
	if r.Active == path {
		r.Active = ""
	}
	return Save(r)
}

// SetActive records which workspace is active.
func SetActive(path string) error {
	r, err := Load()
	if err != nil {
		return err
	}
	r.Active = path
	return Save(r)
}

// List returns the registered workspaces.
func List() ([]Entry, error) {
	r, err := Load()
	if err != nil {
		return nil, err
	}
	return r.Workspaces, nil
}
