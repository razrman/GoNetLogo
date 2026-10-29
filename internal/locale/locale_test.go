package locale

import (
	"os"
	"testing"
)

func TestNormalizeReplacesCAndInvalidTags(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"lang and lc_all C", map[string]string{"LANG": "C", "LC_ALL": "C"}},
		{"posix", map[string]string{"LANG": "POSIX", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"c.utf-8", map[string]string{"LANG": "C.UTF-8", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"c.utf8", map[string]string{"LANG": "C.utf8", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"unknown tag", map[string]string{"LANG": "xx", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"language list starts with C", map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "", "LANGUAGE": "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			if !Normalize() {
				t.Fatal("expected the locale to be replaced")
			}
			if got := os.Getenv("LANG"); got != fallback {
				t.Fatalf("LANG = %q", got)
			}
			if got := os.Getenv("LC_ALL"); got != fallback {
				t.Fatalf("LC_ALL = %q", got)
			}
			if got := os.Getenv("LC_MESSAGES"); got != fallback {
				t.Fatalf("LC_MESSAGES = %q", got)
			}
		})
	}
}

func TestNormalizeLeavesValidLocales(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"en_US", map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"french", map[string]string{"LANG": "fr_FR.UTF-8", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"language list", map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "", "LANGUAGE": "de:fr"}},
		{"lc_messages wins over lang", map[string]string{"LANG": "C", "LC_ALL": "", "LC_MESSAGES": "de_DE.UTF-8", "LANGUAGE": ""}},
		{"empty", map[string]string{"LANG": "", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
		{"chinese", map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			before := snapshot()
			if Normalize() {
				t.Fatal("valid locale was replaced")
			}
			after := snapshot()
			for _, key := range []string{"LANG", "LC_ALL", "LC_MESSAGES", "LANGUAGE"} {
				if before[key] != after[key] {
					t.Fatalf("%s changed from %q to %q", key, before[key], after[key])
				}
			}
		})
	}
}

func TestEffectiveTagMatchesGoLocale(t *testing.T) {
	setEnv(t, map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": "ja_JP:fr"})
	tag, kind := effectiveTag()
	if kind != kindTag || tag != "ja-JP" {
		t.Fatalf("tag %q kind %d", tag, kind)
	}
	setEnv(t, map[string]string{"LANG": "C.UTF-8", "LC_ALL": "", "LC_MESSAGES": "", "LANGUAGE": ""})
	tag, kind = effectiveTag()
	if kind != kindC || tag != "C" {
		t.Fatalf("C.UTF-8 tag %q kind %d", tag, kind)
	}
}

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{"LANG", "LC_ALL", "LC_MESSAGES", "LANGUAGE"} {
		if v, ok := env[key]; ok {
			t.Setenv(key, v)
		}
	}
}

func snapshot() map[string]string {
	out := map[string]string{}
	for _, key := range []string{"LANG", "LC_ALL", "LC_MESSAGES", "LANGUAGE"} {
		out[key] = osGetenv(key)
	}
	return out
}

func osGetenv(key string) string {
	return os.Getenv(key)
}
