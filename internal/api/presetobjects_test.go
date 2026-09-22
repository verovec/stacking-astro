package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verove-jordan/astronomy/internal/preset"
	"github.com/verove-jordan/astronomy/internal/store"
)

// TestListPresets_WireShape pins what GET /api/presets puts on the wire for the object-type picker.
// The handler writes preset.Item values straight through writeJSON with no re-projection, so the
// marshal below IS the response shape; only the user-row projection is the handler's own code, and
// that is exercised directly (it needs no database).
func TestListPresets_WireShape(t *testing.T) {
	t.Run("every built-in exposes a non-empty objects array", func(t *testing.T) {
		raw, err := json.Marshal(preset.Builtins())
		require.NoError(t, err)

		var items []map[string]any
		require.NoError(t, json.Unmarshal(raw, &items))
		require.NotEmpty(t, items)

		for _, it := range items {
			name, _ := it["name"].(string)
			objs, ok := it["objects"].([]any)
			assert.True(t, ok && len(objs) > 0, "built-in %q must serialize objects, got %v", name, it["objects"])
		}
	})

	// A user preset is untagged by design. "Cleanly" matters: a null would force the frontend to
	// distinguish null from absent before it could filter, and that is exactly the kind of
	// three-state check that turns into a bug the first time someone forgets it.
	t.Run("a user preset omits objects rather than sending null", func(t *testing.T) {
		item := userPresetItem(store.Preset{
			ID: 7, Name: "my recipe", Payload: []byte(`{"mode":"deepsky"}`),
			Favorite: true, CreatedAt: 1, UpdatedAt: 2,
		})
		assert.Empty(t, item.Objects)
		assert.False(t, item.Builtin)
		assert.True(t, item.Favorite)

		raw, err := json.Marshal(item)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(raw, &got))
		_, present := got["objects"]
		assert.False(t, present, "objects must be absent for a user preset, got %s", raw)
	})
}
