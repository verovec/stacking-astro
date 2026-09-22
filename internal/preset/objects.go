// objects.go owns the object-type taxonomy: what the user photographed, as opposed to how the engine
// processes it (that is mode.Mode) or where the preset sits in the picker (that is Category).
//
// It lives HERE, in Go, and travels to the UI on the wire with each preset. The alternative — a second
// list in the frontend mapping types to presets — is the mistake internal/filters documents at length:
// two copies of a vocabulary drift, and the drift is invisible until a user lands on the half nobody
// updated. The frontend renders what GET /api/presets serves and declares no taxonomy of its own.
//
// The types are what an astrophotographer would say they shot, not a catalogue morphology: the point
// is to let someone who knows "I shot the Veil" reach the right recipe without knowing what
// `linear_fit` rejection is.
package preset

// ObjectType is one kind of photographic target. Closed enum — see ObjectTypes / Valid.
type ObjectType string

const (
	ObjectGalaxy           ObjectType = "galaxy"
	ObjectEmissionNebula   ObjectType = "emission-nebula"
	ObjectPlanetaryNebula  ObjectType = "planetary-nebula"
	ObjectReflectionNebula ObjectType = "reflection-nebula"
	ObjectDarkNebula       ObjectType = "dark-nebula"
	ObjectSupernovaRemnant ObjectType = "supernova-remnant"
	// ObjectOxygenCloud is an [OIII]-dominant object (OU4's squid, big OIII shells). It is separate
	// from the emission nebula because the processing differs at the root: OIII is the faintest,
	// most light-pollution-sensitive channel, so it is weighted and screened differently.
	ObjectOxygenCloud ObjectType = "oxygen-cloud"
	ObjectStarCluster ObjectType = "star-cluster"
	ObjectComet       ObjectType = "comet"
	ObjectPlanet      ObjectType = "planet"
	ObjectMoon        ObjectType = "moon"
	ObjectSun         ObjectType = "sun"
	ObjectMilkyway    ObjectType = "milkyway"
)

// objectTypeOrder is the display order for the picker's chips — deep-sky targets first, roughly from
// the most commonly shot to the most specialised, then the solar-system and wide-field ones. Slice
// order IS the UI order; the frontend never re-sorts.
var objectTypeOrder = []ObjectType{
	ObjectGalaxy,
	ObjectEmissionNebula,
	ObjectPlanetaryNebula,
	ObjectReflectionNebula,
	ObjectDarkNebula,
	ObjectSupernovaRemnant,
	ObjectOxygenCloud,
	ObjectStarCluster,
	ObjectComet,
	ObjectMoon,
	ObjectPlanet,
	ObjectSun,
	ObjectMilkyway,
}

// ObjectTypes returns every object type in display order. The returned slice is a copy, so a caller
// cannot reorder the catalog for everyone else.
func ObjectTypes() []ObjectType {
	out := make([]ObjectType, len(objectTypeOrder))
	copy(out, objectTypeOrder)
	return out
}

// Valid reports whether o is a known object type. Exact match only: the value is a wire token, and
// tolerating "Galaxy" or "galaxie" here would let a typo'd tag reach the UI and quietly select nothing.
func (o ObjectType) Valid() bool {
	for _, known := range objectTypeOrder {
		if o == known {
			return true
		}
	}
	return false
}

// objects is the authoring helper for a built-in's taxonomy tags, so the catalog reads as a list of
// recipes rather than a list of slice literals.
func objects(o ...ObjectType) []ObjectType { return o }
