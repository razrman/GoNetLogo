package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxModelBytes is the largest model file the server will accept,
// whether the client uploads the text or the file is read from MODELS_DIR.
const maxModelBytes = 8 << 20

// ModelsDir is the only directory the server will read model files from.
// MODELS_DIR overrides the default ./models. The result is absolute, with
// symlinks resolved, so a later path check compares real locations.
func ModelsDir() (string, error) {
	dir := strings.TrimSpace(os.Getenv("MODELS_DIR"))
	if dir == "" {
		dir = "models"
	}
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

// readModelFile reads a .nlogo or .nlogox file from ModelsDir.
// path must be relative. Any ".." segment, an absolute path, or a symlink
// that resolves outside the directory is rejected before the file is used.
func readModelFile(path string) (string, string, error) {
	if err := rejectModelPath(path); err != nil {
		return "", "", err
	}
	root, err := ModelsDir()
	if err != nil {
		return "", "", err
	}
	rel := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")))
	if rel == "." || rel == "" {
		return "", "", fmt.Errorf("model path is empty")
	}
	full := filepath.Join(root, rel)
	if !pathInside(root, full) {
		return "", "", fmt.Errorf("model path escapes the models directory")
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", "", fmt.Errorf("unknown model %q", path)
	}
	resolved = filepath.Clean(resolved)
	if !pathInside(root, resolved) {
		return "", "", fmt.Errorf("model path escapes the models directory")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("unknown model %q", path)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("model path is not a file")
	}
	ext := strings.ToLower(filepath.Ext(resolved))
	if ext != ".nlogo" && ext != ".nlogox" {
		return "", "", fmt.Errorf("model file must be .nlogo or .nlogox")
	}
	if info.Size() > maxModelBytes {
		return "", "", fmt.Errorf("model file is too large")
	}
	b, err := os.ReadFile(resolved)
	if err != nil {
		return "", "", fmt.Errorf("unknown model %q", path)
	}
	if len(b) > maxModelBytes {
		return "", "", fmt.Errorf("model file is too large")
	}
	return filepath.Base(rel), string(b), nil
}

func rejectModelPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("model path is empty")
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("model path is invalid")
	}
	slash := strings.ReplaceAll(path, "\\", "/")
	if isAbsModelPath(slash) {
		return fmt.Errorf("model path must stay inside the models directory")
	}
	for _, part := range strings.Split(slash, "/") {
		if part == ".." {
			return fmt.Errorf("model path escapes the models directory")
		}
	}
	return nil
}

func isAbsModelPath(slash string) bool {
	if strings.HasPrefix(slash, "/") {
		return true
	}
	if len(slash) >= 2 && slash[1] == ':' {
		c := slash[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return false
}

func pathInside(root, full string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(full))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// modelDisplayName keeps the file's base name for the session label.
// Directory components are dropped so a client cannot smuggle a path.
func modelDisplayName(filename string) string {
	filename = strings.TrimSpace(filename)
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)
	filename = strings.ReplaceAll(filename, "\\", "/")
	if i := strings.LastIndex(filename, "/"); i >= 0 {
		filename = filename[i+1:]
	}
	filename = strings.TrimSpace(filename)
	if filename == "" || filename == "." || filename == ".." {
		return "model.nlogo"
	}
	const maxName = 180
	if len(filename) > maxName {
		ext := filepath.Ext(filename)
		if len(ext) >= maxName {
			ext = ext[:maxName]
		}
		filename = filename[:maxName-len(ext)] + ext
	}
	return filename
}
