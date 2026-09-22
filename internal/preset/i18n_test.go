package preset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// i18nLocales are the UI's translation files, relative to this package.
var i18nLocales = []string{
	filepath.Join("..", "..", "frontend", "src", "i18n", "en.json"),
	filepath.Join("..", "..", "frontend", "src", "i18n", "fr.json"),
}

// loadPresetI18n returns the `preset` subtree of a locale file.
func loadPresetI18n(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("locale %s unavailable: %v", path, err) // engine-only checkouts have no frontend tree
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
	sub, ok := doc["preset"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no preset subtree", path)
	}
	return sub
}

// TestObjectTypes_AreTranslated closes the loop between the Go taxonomy and the UI that renders it.
// The types are single-sourced here and travel on the wire, which keeps the VALUES from drifting —
// but a type added in Go with no matching label would still reach the picker as a raw slug. This
// fails at the source instead, in the package that owns the list.
func TestObjectTypes_AreTranslated(t *testing.T) {
	for _, locale := range i18nLocales {
		t.Run(filepath.Base(locale), func(t *testing.T) {
			p := loadPresetI18n(t, locale)
			objects, ok := p["objects"].(map[string]any)
			if !ok {
				t.Fatalf("%s: preset.objects is missing", locale)
			}
			for _, o := range ObjectTypes() {
				label, present := objects[string(o)]
				if !present {
					t.Errorf("object type %q has no label in %s", o, locale)
					continue
				}
				if s, _ := label.(string); s == "" {
					t.Errorf("object type %q has an empty label in %s", o, locale)
				}
			}
			for key := range objects {
				if !ObjectType(key).Valid() {
					t.Errorf("%s translates %q, which is not an object type", locale, key)
				}
			}
		})
	}
}

// TestBuiltins_AreTranslated pins the other half of the catalog contract: every built-in slug keys a
// label + desc the picker shows. A new recipe with no strings renders as its raw slug.
func TestBuiltins_AreTranslated(t *testing.T) {
	for _, locale := range i18nLocales {
		t.Run(filepath.Base(locale), func(t *testing.T) {
			p := loadPresetI18n(t, locale)
			entries, ok := p["builtin"].(map[string]any)
			if !ok {
				t.Fatalf("%s: preset.builtin is missing", locale)
			}
			for _, it := range Builtins() {
				entry, present := entries[it.Name].(map[string]any)
				if !present {
					t.Errorf("built-in %q has no translation in %s", it.Name, locale)
					continue
				}
				for _, field := range []string{"label", "desc"} {
					if s, _ := entry[field].(string); s == "" {
						t.Errorf("built-in %q has an empty %s in %s", it.Name, field, locale)
					}
				}
			}
		})
	}
}

// TestPresetCategories_AreTranslated catches the failure mode that hid the five sun recipes: a
// category the engine serves but the UI has no name for.
func TestPresetCategories_AreTranslated(t *testing.T) {
	for _, locale := range i18nLocales {
		t.Run(filepath.Base(locale), func(t *testing.T) {
			p := loadPresetI18n(t, locale)
			names, ok := p["category"].(map[string]any)
			if !ok {
				t.Fatalf("%s: preset.category is missing", locale)
			}
			for _, it := range Builtins() {
				if s, _ := names[it.Category].(string); s == "" {
					t.Errorf("category %q (used by %q) has no name in %s", it.Category, it.Name, locale)
				}
			}
		})
	}
}
