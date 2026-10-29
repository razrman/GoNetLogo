// Package locale picks a process locale Fyne 2.5.4 can parse.
//
// Fyne reads the locale from inside lang.init, which runs before main and
// before any init in a package that imports Fyne. This package must not
// import Fyne. The desktop GUI imports it so this init runs first and a
// C, POSIX, or otherwise invalid tag is replaced before Fyne logs
// "Error parsing user locale".
package locale

import (
	"os"
	"strings"

	"golang.org/x/text/language"
)

// fallback is a common UTF-8 locale. Fyne accepts the tag it parses to.
const fallback = "en_US.UTF-8"

func init() {
	Normalize()
}

// Normalize sets the process locale to en_US.UTF-8 when the tag Fyne would
// parse is C, POSIX, or not valid BCP 47. A tag Fyne already accepts is
// left alone, including an empty locale (Fyne falls back to English without
// logging). It returns whether the environment changed.
func Normalize() bool {
	tag, kind := effectiveTag()
	switch kind {
	case kindEmpty:
		return false
	case kindTag:
		if _, err := language.Parse(tag); err == nil {
			return false
		}
	}
	applyFallback()
	return true
}

type kind int

const (
	kindEmpty kind = iota
	kindC
	kindTag
)

// effectiveTag follows github.com/jeandeaual/go-locale's Unix rules, which
// is what Fyne 2.5.4 calls. LC_ALL, then LC_MESSAGES, then LANG. C and POSIX
// stop the search. Otherwise LANGUAGE wins when it is set. The returned tag
// is the first item, with the encoding removed and underscores turned into
// the hyphen go-locale hands to language.Parse.
func effectiveTag() (string, kind) {
	raw := ""
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		raw = os.Getenv(key)
		if raw != "" {
			break
		}
	}
	if raw == "C" || raw == "POSIX" {
		return raw, kindC
	}
	if languages := os.Getenv("LANGUAGE"); languages != "" {
		raw = languages
	}
	if raw == "" {
		return "", kindEmpty
	}
	if raw == "C" || raw == "POSIX" {
		return raw, kindC
	}
	first, _, _ := strings.Cut(raw, ":")
	tag := bcp47(first)
	if tag == "C" || tag == "POSIX" || tag == "" {
		return tag, kindC
	}
	return tag, kindTag
}

func bcp47(unix string) string {
	formatted, _, _ := strings.Cut(unix, ".")
	at, _, _ := strings.Cut(formatted, "@")
	formatted = at
	formatted = strings.ReplaceAll(formatted, "-", "_")
	lang, territory, _ := strings.Cut(formatted, "_")
	if territory != "" {
		return lang + "-" + territory
	}
	return lang
}

func applyFallback() {
	_ = os.Setenv("LANG", fallback)
	_ = os.Setenv("LC_ALL", fallback)
	_ = os.Setenv("LC_MESSAGES", fallback)
	// go-locale prefers LANGUAGE over a non-C LC_ALL. Leaving a bad
	// LANGUAGE in place would make the fallback invisible to Fyne.
	if os.Getenv("LANGUAGE") != "" {
		_ = os.Setenv("LANGUAGE", "en_US")
	}
}
