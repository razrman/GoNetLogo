package session

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gonetlogo/models"
)

// maxModelBytes is the largest model file the server will accept,
// whether the client uploads the text or the file is read from MODELS_DIR.
const maxModelBytes = 8 << 20

// ModelsDir is the only directory the server will read model files from.
// MODELS_DIR overrides discovery. Otherwise the models directory is found
// from the working directory or the executable, including a macOS .app.
// The result is absolute, with symlinks resolved, so a later path check
// compares real locations.
func ModelsDir() (string, error) {
	return models.ModelsRoot()
}

// LibraryEntry is one file or folder in the Open browser.
type LibraryEntry struct {
	Name      string
	Path      string
	Directory bool
}

// LibraryListing is one folder inside ModelsDir.
type LibraryListing struct {
	Path      string
	Parent    *string
	Entries   []LibraryEntry
	Available bool
}

// OpenPath is the default Open folder relative to ModelsDir.
// ok is false when that folder is missing or sits outside the models
// directory. The desktop dialog may still open an outside
// GONETLOGO_MODELS_DIR; the server will not read it.
func OpenPath() (string, bool) {
	root, err := ModelsDir()
	if err != nil {
		return "", false
	}
	open, ok := models.OpenDir()
	if !ok || !pathInside(root, open) {
		return "", false
	}
	rel, err := filepath.Rel(root, open)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	if rel == "." {
		return "", true
	}
	return filepath.ToSlash(rel), true
}

// ListFolder lists model files and subfolders of one directory inside ModelsDir.
// rel is relative. An empty rel lists the models root. A missing models
// directory returns Available false and no error so the client can fall
// back to a local file chooser.
func ListFolder(rel string) (LibraryListing, error) {
	root, err := ModelsDir()
	if err != nil {
		return LibraryListing{}, nil
	}
	rel = strings.TrimSpace(rel)
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if rel == "." {
		rel = ""
	}
	if rel != "" {
		if err := rejectModelPath(rel); err != nil {
			return LibraryListing{}, err
		}
	}
	full := root
	if rel != "" {
		full = filepath.Join(root, filepath.FromSlash(rel))
	}
	if !pathInside(root, full) {
		return LibraryListing{}, fmt.Errorf("model path escapes the models directory")
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return LibraryListing{}, fmt.Errorf("unknown model folder %q", rel)
	}
	resolved = filepath.Clean(resolved)
	if !pathInside(root, resolved) {
		return LibraryListing{}, fmt.Errorf("model path escapes the models directory")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return LibraryListing{}, fmt.Errorf("unknown model folder %q", rel)
	}
	dirEntries, err := os.ReadDir(resolved)
	if err != nil {
		return LibraryListing{}, fmt.Errorf("unknown model folder %q", rel)
	}
	var entries []LibraryEntry
	for _, entry := range dirEntries {
		name := entry.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		target := filepath.Join(resolved, name)
		linked, err := filepath.EvalSymlinks(target)
		if err != nil || !pathInside(root, filepath.Clean(linked)) {
			continue
		}
		st, err := os.Stat(linked)
		if err != nil {
			continue
		}
		itemPath := name
		if rel != "" {
			itemPath = rel + "/" + name
		}
		if st.IsDir() {
			entries = append(entries, LibraryEntry{Name: name, Path: itemPath, Directory: true})
			continue
		}
		if st.Mode().IsRegular() && isLibraryModel(name) {
			entries = append(entries, LibraryEntry{Name: name, Path: itemPath, Directory: false})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	listing := LibraryListing{Path: rel, Entries: entries, Available: true}
	if rel != "" {
		parent := path.Dir(rel)
		if parent == "." {
			parent = ""
		}
		listing.Parent = &parent
	}
	return listing, nil
}

func isLibraryModel(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".nlogo", ".nlogox", ".nlogo3d", ".nlogox3d":
		return true
	default:
		return false
	}
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
	if !isLibraryModel(filepath.Base(resolved)) {
		return "", "", fmt.Errorf("model file must be .nlogo, .nlogox, .nlogo3d, or .nlogox3d")
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
