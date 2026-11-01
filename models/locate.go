package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// How far above the working directory or the executable to look for models.
// A macOS bundle is bin/GoNetLogo.app/Contents/MacOS/<binary>, so the repo
// root is four directories up from the executable. Eight covers that and a
// launch from bin/ without walking the whole disk.
const modelsSearchDepth = 8

// ModelsRoot is the directory of demo models and the bundled library.
// MODELS_DIR overrides discovery when it is set. Otherwise the search
// starts at the working directory and the executable (including
// Contents/Resources inside a macOS .app) and walks upward for a
// directory that contains fire.nlogo or library/Sample Models.
// The result is absolute, with symlinks resolved.
func ModelsRoot() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("MODELS_DIR")); dir != "" {
		return resolveDir(dir)
	}
	if dir, ok := discoverModelsRoot(searchStarts()); ok {
		return dir, nil
	}
	// Last resort: ./models from the working directory, which is what
	// callers used before discovery. A missing folder still errors.
	return resolveDir("models")
}

// OpenDir is the folder the Open command should start in.
// GONETLOGO_MODELS_DIR wins when it names an existing directory. A missing
// override is ignored and the search continues. The next choice is
// <models root>/library when that directory exists, then the models root.
// ok is false when no models directory can be found.
func OpenDir() (string, bool) {
	if dir := strings.TrimSpace(os.Getenv("GONETLOGO_MODELS_DIR")); dir != "" {
		if resolved, err := resolveDir(dir); err == nil {
			return resolved, true
		}
	}
	root, err := ModelsRoot()
	if err != nil {
		return "", false
	}
	lib := filepath.Join(root, "library")
	if info, err := os.Stat(lib); err == nil && info.IsDir() {
		if resolved, err := filepath.EvalSymlinks(lib); err == nil {
			return filepath.Clean(resolved), true
		}
		return filepath.Clean(lib), true
	}
	return root, true
}

func resolveDir(dir string) (string, error) {
	if strings.ContainsRune(dir, 0) {
		return "", fmt.Errorf("MODELS_DIR is invalid")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("models directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("models directory %s: %w", abs, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("models directory %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("models directory %s is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func searchStarts() []string {
	var starts []string
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		starts = append(starts, cwd)
	}
	exe, err := os.Executable()
	if err != nil {
		return starts
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	exeDir := filepath.Dir(exe)
	starts = append(starts, exeDir)
	// GoNetLogo.app/Contents/MacOS/<bin> → Contents/Resources
	starts = append(starts, filepath.Clean(filepath.Join(exeDir, "..", "Resources")))
	return starts
}

func discoverModelsRoot(starts []string) (string, bool) {
	seen := map[string]bool{}
	for _, start := range starts {
		if start == "" {
			continue
		}
		dir := filepath.Clean(start)
		for i := 0; i < modelsSearchDepth; i++ {
			if seen[dir] {
				break
			}
			seen[dir] = true
			if found, ok := acceptModelsRoot(dir); ok {
				return found, true
			}
			if found, ok := acceptModelsRoot(filepath.Join(dir, "models")); ok {
				return found, true
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", false
}

func acceptModelsRoot(path string) (string, bool) {
	if !isModelsRoot(path) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path), true
	}
	return filepath.Clean(resolved), true
}

func isModelsRoot(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	if st, err := os.Stat(filepath.Join(path, "fire.nlogo")); err == nil && st.Mode().IsRegular() {
		return true
	}
	st, err := os.Stat(filepath.Join(path, "library", "Sample Models"))
	return err == nil && st.IsDir()
}
