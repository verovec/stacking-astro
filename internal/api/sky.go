package api

import (
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/verove-jordan/astronomy/internal/skycat"
	"github.com/verove-jordan/astronomy/internal/skyplan"
)

// skySearchResult is one /api/sky/search hit — the catalogue record projected onto the wire.
type skySearchResult struct {
	Name             string   `json:"name"`
	RADeg            float64  `json:"ra_deg"`
	DecDeg           float64  `json:"dec_deg"`
	Type             string   `json:"type,omitempty"`
	Source           string   `json:"source,omitempty"`
	SizeArcmin       *float64 `json:"size_arcmin,omitempty"`
	SizeMinorArcmin  *float64 `json:"size_minor_arcmin,omitempty"`
	PositionAngleDeg *float64 `json:"position_angle_deg,omitempty"`
	Mag              *float64 `json:"mag,omitempty"`
	Morphology       string   `json:"morphology,omitempty"`
	CommonNames      []string `json:"common_names,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
}

// skySearch does free-text lookup over the WHOLE merged deep-sky catalogue, so the mosaic planner can
// target any object by name or alias.
// GET /api/sky/search?q=&limit=
func (s *Server) skySearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		badRequest(w, "missing 'q'")
		return
	}
	limit := clampAtoi(r.URL.Query().Get("limit"), 20, 1, 100)
	recs := skycat.Load(s.cfg.SirilCatalogDir).Search(query, limit)

	out := make([]skySearchResult, 0, len(recs))
	for _, rec := range recs {
		out = append(out, skySearchResultFor(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out, "count": len(out)})
}

// skySearchResultFor projects a catalogue record onto the wire shape, reusing skyplan.DeriveType so the
// type vocabulary stays the skyplan-derived one.
func skySearchResultFor(rec skycat.Record) skySearchResult {
	res := skySearchResult{
		Name:        rec.Name,
		RADeg:       rec.RADeg,
		DecDeg:      rec.DecDeg,
		Type:        skyplan.DeriveType(rec),
		Source:      rec.Source,
		Morphology:  rec.Morphology,
		CommonNames: rec.CommonNames,
		Aliases:     rec.Aliases,
	}
	if rec.HasDiameter {
		d := rec.DiameterArcmin
		res.SizeArcmin = &d
	}
	if rec.HasMinorAxis {
		m := rec.MinorAxisArcmin
		res.SizeMinorArcmin = &m
	}
	if rec.HasPositionAngle {
		pa := rec.PositionAngleDeg
		res.PositionAngleDeg = &pa
	}
	if rec.HasMag {
		mag := rec.MagV
		res.Mag = &mag
	}
	return res
}

func floatParam(q url.Values, key string, def float64) float64 {
	if v := q.Get(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func intParam(q url.Values, key string, def int) int {
	if v := q.Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func twilightParam(v string) string {
	if v == "nautical" {
		return "nautical"
	}
	return "astro"
}

// modeParam normalizes the observing-mode query value to "visual" or "camera" (the default).
func modeParam(v string) string {
	if v == "visual" {
		return "visual"
	}
	return "camera"
}

// eyepiecesParam parses a comma-separated visual kit of "focalMM:afovDeg[:label]" items
// (e.g. "30:68:30mm,10:60"). Malformed or non-positive entries are skipped; a missing label defaults
// to "{focal}mm".
func eyepiecesParam(raw string) []skyplan.Eyepiece {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var kit []skyplan.Eyepiece
	for _, item := range strings.Split(raw, ",") {
		fields := strings.Split(item, ":")
		if len(fields) < 2 {
			continue
		}
		focal, errFocal := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
		afov, errAFOV := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
		if errFocal != nil || errAFOV != nil || focal <= 0 || afov <= 0 {
			continue
		}
		label := ""
		if len(fields) >= 3 {
			label = strings.TrimSpace(fields[2])
		}
		if label == "" {
			label = strconv.FormatFloat(focal, 'f', -1, 64) + "mm"
		}
		kit = append(kit, skyplan.Eyepiece{FocalMM: focal, AFOVDeg: afov, Label: label})
	}
	return kit
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}
