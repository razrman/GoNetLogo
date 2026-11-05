package nl

import (
	"fmt"
	"os"
	"path/filepath"
)

// expandIncludes splices __includes files into src. Paths are relative to
// dir, which is the directory of the file that contains the directive.
// Included files are parsed with the model, so their procedures and
// declarations are real code rather than skipped text. A missing file or
// a cycle is an error.
func expandIncludes(src, dir string, seen map[string]bool) (string, error) {
	if seen == nil {
		seen = map[string]bool{}
	}
	toks, err := cleanTokens(src)
	if err != nil {
		return "", err
	}
	var b []byte
	cursor := 0
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t.Kind != TokIdent || t.Lit != "__includes" {
			i++
			continue
		}
		if t.Pos.Offset < cursor || t.Pos.Offset > len(src) {
			return "", fmt.Errorf("__includes: bad source position")
		}
		b = append(b, src[cursor:t.Pos.Offset]...)
		i++
		if i >= len(toks) || toks[i].Kind != TokLBrack {
			return "", fmt.Errorf("__includes expected a list of file names")
		}
		i++
		var files []string
		for i < len(toks) && toks[i].Kind != TokRBrack {
			ft := toks[i]
			if ft.Kind != TokString {
				return "", fmt.Errorf("__includes expected a file name string, got %s", ft.Text)
			}
			files = append(files, ft.Lit)
			i++
		}
		if i >= len(toks) || toks[i].Kind != TokRBrack {
			return "", fmt.Errorf("__includes expected ]")
		}
		end := toks[i].Pos.Offset + 1
		if end > len(src) {
			end = len(src)
		}
		cursor = end
		i++
		if len(files) == 0 {
			continue
		}
		if dir == "" {
			return "", fmt.Errorf("__includes cannot be loaded without a model directory")
		}
		for _, rel := range files {
			full := rel
			if !filepath.IsAbs(rel) {
				full = filepath.Join(dir, filepath.FromSlash(rel))
			}
			abs, err := filepath.Abs(full)
			if err != nil {
				abs = full
			}
			if seen[abs] {
				return "", fmt.Errorf("__includes: %s is already included", rel)
			}
			seen[abs] = true
			body, err := os.ReadFile(full)
			if err != nil {
				return "", fmt.Errorf("__includes: cannot read %s", rel)
			}
			expanded, err := expandIncludes(string(body), filepath.Dir(abs), seen)
			if err != nil {
				return "", err
			}
			b = append(b, '\n')
			b = append(b, expanded...)
			b = append(b, '\n')
		}
	}
	if cursor < len(src) {
		b = append(b, src[cursor:]...)
	}
	return string(b), nil
}
