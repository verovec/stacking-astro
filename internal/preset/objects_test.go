package preset

import (
	"encoding/json"
	"testing"
)

// TestObjectTypes_ClosedEnum pins the taxonomy as a CLOSED list with one home, in Go. The lesson is
// internal/filters': the moment a second copy of a vocabulary exists (there, the filter list in the
// frontend) the two drift, and the drift is invisible until a user hits the half that was never
// updated. The frontend renders what GET /api/presets serves; it never declares its own types.
func TestObjectTypes_ClosedEnum(t *testing.T) {
	seen := map[ObjectType]bool{}
	for _, o := range ObjectTypes() {
		if !o.Valid() {
			t.Errorf("ObjectTypes() yields %q, which Valid() rejects", o)
		}
		if seen[o] {
			t.Errorf("duplicate object type %q", o)
		}
		seen[o] = true
	}
	if len(seen) == 0 {
		t.Fatal("ObjectTypes() is empty")
	}
	for _, bad := range []ObjectType{"", "nebula", "Galaxy", "galaxie", "star cluster", "deepsky"} {
		if bad.Valid() {
			t.Errorf("%q must not be a valid object type", bad)
		}
	}
}

// TestBuiltins_AllTaggedWithObjects is the guard that makes the picker's guidance trustworthy: an
// untagged builtin silently falls out of every object-type filter, which reads to the user as "there
// is no preset for a galaxy" rather than "someone forgot a tag".
func TestBuiltins_AllTaggedWithObjects(t *testing.T) {
	for _, it := range Builtins() {
		t.Run(it.Name, func(t *testing.T) {
			if len(it.Objects) == 0 {
				t.Fatalf("built-in %q carries no object type", it.Name)
			}
			seen := map[ObjectType]bool{}
			for _, o := range it.Objects {
				if !o.Valid() {
					t.Errorf("built-in %q carries unknown object type %q", it.Name, o)
				}
				if seen[o] {
					t.Errorf("built-in %q repeats object type %q", it.Name, o)
				}
				seen[o] = true
			}
		})
	}
}

// TestBuiltins_EveryObjectTypeHasAPreset closes the loop the other way: a chip the user can pick must
// lead somewhere. A type with no preset behind it is a dead end in the UI.
func TestBuiltins_EveryObjectTypeHasAPreset(t *testing.T) {
	covered := map[ObjectType]int{}
	for _, it := range Builtins() {
		for _, o := range it.Objects {
			covered[o]++
		}
	}
	for _, o := range ObjectTypes() {
		if covered[o] == 0 {
			t.Errorf("object type %q has no built-in preset — the chip would select nothing", o)
		}
	}
}

// TestItem_ObjectsSerialization pins the wire contract the frontend reads: built-ins expose "objects",
// and a user-saved preset (which has no taxonomy) omits the key entirely rather than sending null.
func TestItem_ObjectsSerialization(t *testing.T) {
	t.Run("built-in exposes objects", func(t *testing.T) {
		raw, err := json.Marshal(Builtins()[0])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		list, ok := got["objects"].([]any)
		if !ok || len(list) == 0 {
			t.Fatalf("built-in must serialize a non-empty objects array, got %s", raw)
		}
	})

	t.Run("user preset omits objects", func(t *testing.T) {
		raw, err := json.Marshal(Item{ID: 7, Name: "mine", Payload: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, present := got["objects"]; present {
			t.Errorf("a user preset must omit objects entirely, got %s", raw)
		}
	})
}

// TestBuiltins_NewRecipesPresent pins the five entries this card adds. They are named individually
// because each closes a documented gap (stacking-doc §8.3, the OSC-broadband runbook, and the
// measured wide-field-lens run) — a rename or an accidental removal should fail loudly.
func TestBuiltins_NewRecipesPresent(t *testing.T) {
	want := map[string][]ObjectType{
		"oxygen-cloud":      {ObjectOxygenCloud},
		"supernova-remnant": {ObjectSupernovaRemnant},
		"dark-nebula":       {ObjectDarkNebula},
		"osc-broadband":     nil, // colour-rig recipe: spans several target types
		"wide-field-lens":   nil, // optics-driven: whatever a camera lens frames
	}
	got := map[string][]ObjectType{}
	for _, it := range Builtins() {
		got[it.Name] = it.Objects
	}
	for slug, mustHave := range want {
		objs, ok := got[slug]
		if !ok {
			t.Errorf("missing built-in %q", slug)
			continue
		}
		for _, need := range mustHave {
			if !containsObject(objs, need) {
				t.Errorf("built-in %q must be tagged %q, got %v", slug, need, objs)
			}
		}
	}
}

func containsObject(list []ObjectType, want ObjectType) bool {
	for _, o := range list {
		if o == want {
			return true
		}
	}
	return false
}
