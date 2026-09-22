// Package config loads runtime configuration from the environment.
package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/verove-jordan/astronomy/internal/starnet"
)

// Config holds all runtime configuration for the engine.
type Config struct {
	DatabaseURL string
	APIAddr     string
	LogLevel    string

	DataDir    string // root the UI may browse for capture folders
	WorkDir    string // scratch space for intermediate FITS / sequences
	KeepWork   bool   // keep run scratch after terminal jobs (debugging) — disables the work sweep
	OutputDir  string // where final stacks and reports are written
	LibraryDir string // persistent master-calibration library

	// BrowseRoots are EXTRA absolute roots the UI may browse for external drives, on top of the platform
	// removable-media defaults (macOS /Volumes; Linux /media, /mnt, /run/media). Colon- or comma-separated
	// in ASTRO_BROWSE_ROOTS. Every /api/local/* handler confines its paths to these roots + the defaults.
	BrowseRoots []string

	// PreviewMaxEdge caps the longest edge (px) of the in-browser file-preview buffer the API decodes
	// for the file viewer; smaller = less memory/transfer, larger = more detail when zooming.
	PreviewMaxEdge int

	SirilBin  string
	GimpBin   string
	GimpHost  string
	GimpPort  int
	FfmpegBin string

	// Optional astro-AI host tools (invoked like Siril/GIMP, never bundled). Empty/missing →
	// the pipeline falls back to Siril (subsky) and skips star removal.
	GraxpertBin   string // GraXpert: AI background-gradient extraction + denoise
	GraxpertURL   string // optional host GraXpert HTTP service (cmd/graxpert-host); empty → exec GraxpertBin locally
	GraxpertGPU   bool   // pass -gpu true (helps background-extraction on Apple Silicon; denoise stays CPU — its model is CoreML-incompatible)
	GraxpertBatch int    // GraXpert denoise -batch_size (tiles denoised in parallel; 0 → GraXpert's default of 4)
	// DenoiseScale (0,1) runs the joint AI colour denoise on a downscaled copy and transfers only
	// the chroma back (luminance untouched, ~scale² of the cost — best for LRGB where L carries
	// detail). 1.0 (the default) keeps the full-resolution pass byte-identical.
	DenoiseScale float64
	// ChannelParallel stacks up to N deep-sky channels concurrently (each Siril instance gets an
	// equal share of the CPU/memory budget). 1 (the default) keeps the proven serial loop.
	ChannelParallel int
	// StarnetBin is the host StarNet used for star removal (star-reduced finishing + the star-tier
	// deliverables). Unset → the first of starnet.DefaultBinCandidates found on PATH, so either CLI
	// generation is picked up without configuration.
	StarnetBin string
	// StarnetCLI pins how that binary is invoked — "positional" (StarNet++ v2) or "flags" (StarNet2
	// v2.5+). Empty/"auto" (the default) probes the binary itself; set it only to override a
	// misdetection.
	StarnetCLI string

	// Optional local LLM "supervisor" (opt-in via the run request / --supervise). The engine drives a
	// host-run, OpenAI-compatible model server (LM Studio / mlx-vlm) over HTTP to auto-tune the finish.
	// An empty URL or an unreachable server → the run uses the normal single-pass finish.
	LLMBaseURL     string        // OpenAI-compatible base, e.g. http://127.0.0.1:1234/v1
	LLMModel       string        // chat/vision model id served there
	LLMImageFormat string        // vision wire-format: "openai" (default) or "mlxvlm"
	LLMTimeout     time.Duration // max wall-clock for one chat/vision completion; 0 → no limit
	// LLMAssistPromptExtra is appended to the AstroAgent chat system prompt (tone/policy tweaks without
	// recompiling); the grounding rules + knob menu stay fixed in code.
	LLMAssistPromptExtra string

	// Resource limits keep a heavy stack from freezing the host (Siril defaults to all cores and
	// 90% of RAM, which thrashes swap). MaxCPUs caps Siril's threads (setcpu); SirilMemRatio caps
	// the fraction of available RAM it may use (setmem); SirilNice lowers siril-cli OS priority;
	// MaxWorkers bounds concurrent jobs in the API worker pool (0 → runtime.NumCPU()/2).
	MaxCPUs       int
	SirilMemRatio float64
	SirilNice     int
	MaxWorkers    int

	// Plate-solving + SPCC (color calibration). Focal/pixel describe the rig (the FITS rarely
	// carries FOCALLEN); the SPCC names must match Siril's catalogs. Empty values fall back to
	// Siril defaults. PlateSolveCatalog empty → Siril chooses automatically.
	FocalLenMM  float64
	PixelSizeUm float64

	SpccMonoSensor string
	SpccRFilter    string
	SpccGFilter    string
	SpccBFilter    string
	SpccWhiteRef   string
	// NightscapeOSCSensor is the SPCC OSC sensor name for the milkyway/nightscape path (the one-shot
	// camera, e.g. a DSLR). Empty (the default) disables SPCC for nightscapes — a phone sensor is rarely
	// in Siril's SPCC database — so the run uses the background-neutralization colour path instead.
	NightscapeOSCSensor string
	PlateSolveCatalog   string
	SirilCatalogDir     string // Siril's bundled object catalogues (for name→coords resolution)

	// Local Gaia DR3 catalogues (downloaded once via `just download-catalogues[-spcc]`) make
	// plate-solving and SPCC work fully offline. GaiaAstroCat is the astrometric extract FILE;
	// GaiaXpsampDir is the DIRECTORY holding the xp_sampled chunk files. Use the LocalGaia*()
	// accessors, which return them only when the files are actually present. LocalAsnet switches
	// solving to a local astrometry.net install instead. SpccCatalog forces the SPCC source
	// ("gaia" online / "localgaia"); empty lets Siril prefer local when installed.
	GaiaAstroCat  string
	GaiaXpsampDir string
	LocalAsnet    bool
	SpccCatalog   string

	// DeepStarCat is the deep star catalogue (ATHYG v3.2, ~2.5 million stars) the run annotation
	// names field stars from, downloaded once via `just download-deepstars`. OPTIONAL: when the file
	// is absent the annotation falls back to the embedded magnitude-9 extract, which names the
	// bright stars and leaves the rest anonymous.
	DeepStarCat string

	// Observing site + rig for the "tonight" visibility planner. Latitude/longitude default to Paris
	// so the page works out of the box; the web UI overrides them per-session (and persists locally).
	// ApertureMM and the sensor dimensions complete the optical setup; FocalLenMM/PixelSizeUm above are
	// reused for image scale and field of view.
	LatDeg     float64
	LonDeg     float64
	ElevationM float64
	Timezone   string // IANA name, e.g. "Europe/Paris"
	ApertureMM float64
	SensorWpx  int
	SensorHpx  int
	// EyepieceKit is the default visual-observing eyepiece set for the tonight planner's eyepiece mode,
	// encoded as "focalMM:apparentFOVdeg[:label]" items separated by commas. The web UI overrides it
	// per-session; an empty value disables the per-target eyepiece recommendation.
	EyepieceKit string
	// BarlowX is the default Barlow/amplifier factor applied to the focal length for the tonight planner
	// (image scale, FOV, f-ratio and eyepiece magnification). 1 means no Barlow.
	BarlowX float64
	// ReducerX is the default focal-reducer factor (e.g. 0.66), applied to the same derived values and
	// independently of BarlowX — a reducer usually stays in the train while a Barlow comes and goes.
	// 1 means no reducer.
	ReducerX float64

	// Cross-session reuse. Reuse pools prior light frames of the same target (to grow integration)
	// and prior raw bias/darks (for deeper, lower-noise masters). ReuseEnabled gates the whole
	// feature; ReuseConeDeg is the coordinate-match radius; ReuseDarkRecencyDays bounds how old a
	// dark may be (0 = unbounded); ReuseTempTolC is the dark temperature tolerance (°C).
	ReuseEnabled         bool
	ReuseConeDeg         float64
	ReuseDarkRecencyDays int
	ReuseTempTolC        float64
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() *Config {
	sirilBin := env("SIRIL_BIN", "/Applications/Siril.app/Contents/MacOS/siril-cli")
	catalogDir := env("ASTRO_SIRIL_CATALOG_DIR", "")
	if catalogDir == "" { // derive from the Siril app bundle (macOS host-engine)
		catalogDir = filepath.Clean(filepath.Join(filepath.Dir(sirilBin), "..", "Resources", "share", "siril", "catalogue"))
	}
	libraryDir := env("ASTRO_LIBRARY_DIR", "./library")
	return &Config{
		DatabaseURL: env("DATABASE_URL", "postgres://astro:astro@localhost:5432/astrostack?sslmode=disable"),
		APIAddr:     env("API_ADDR", ":8080"),
		LogLevel:    env("LOG_LEVEL", "info"),
		// ./input, not ./data: compose.yaml pins the container's ASTRO_DATA_DIR to ${PWD}/input, and
		// two different defaults meant host-dev and the container browsed different roots — the same
		// capture visible in one mode and invisible in the other, with nothing to explain why.
		DataDir:        env("ASTRO_DATA_DIR", "./input"),
		WorkDir:        env("ASTRO_WORK_DIR", "./work"),
		KeepWork:       envBool("ASTRO_KEEP_WORK", false),
		OutputDir:      env("ASTRO_OUTPUT_DIR", "./output"),
		LibraryDir:     libraryDir,
		BrowseRoots:    envStrList("ASTRO_BROWSE_ROOTS"),
		PreviewMaxEdge: envInt("PREVIEW_MAX_EDGE", 1500),
		SirilBin:       sirilBin,
		GimpBin:        env("GIMP_BIN", "/Applications/GIMP.app/Contents/MacOS/gimp-console-2.10"),
		GimpHost:       env("GIMP_HOST", "127.0.0.1"),
		GimpPort:       envInt("GIMP_PORT", 10008),
		FfmpegBin:      env("FFMPEG_BIN", "ffmpeg"),
		// GraXpert/StarNet are resolved via PATH by default (pip/pipx installs land in PATH as
		// `graxpert`); the old default pointed at a GraXpert.app that pip installs don't create, so AI
		// background extraction was silently skipped. exec.LookPath accepts a bare name or an abs path.
		GraxpertBin:     env("GRAXPERT_BIN", "graxpert"),
		GraxpertURL:     env("ASTRO_GRAXPERT_URL", ""),
		GraxpertGPU:     envBool("ASTRO_GRAXPERT_GPU", false),
		GraxpertBatch:   envInt("ASTRO_GRAXPERT_BATCH", 0),
		DenoiseScale:    envFloat("ASTRO_DENOISE_SCALE", 1.0),
		ChannelParallel: envInt("ASTRO_CHANNEL_PARALLEL", 1),
		StarnetBin:      env("STARNET_BIN", firstOnPath(starnet.DefaultBinCandidates)),
		StarnetCLI:      env("STARNET_CLI", string(starnet.VariantAuto)),

		LLMBaseURL:           env("ASTRO_LLM_URL", "http://127.0.0.1:1234/v1"),
		LLMModel:             env("ASTRO_LLM_MODEL", ""),
		LLMImageFormat:       env("ASTRO_LLM_IMAGE_FORMAT", "openai"),
		LLMTimeout:           time.Duration(envInt("ASTRO_LLM_TIMEOUT_SEC", 3600)) * time.Second,
		LLMAssistPromptExtra: env("ASTRO_LLM_ASSIST_PROMPT_EXTRA", ""),

		MaxCPUs:       envInt("ASTRO_MAX_CPUS", 10),
		SirilMemRatio: envFloat("ASTRO_SIRIL_MEM_RATIO", 0.5),
		SirilNice:     envInt("ASTRO_SIRIL_NICE", 10),
		MaxWorkers:    envInt("ASTRO_MAX_WORKERS", 0),

		FocalLenMM:  envFloat("ASTRO_FOCAL_MM", 740), // Takahashi FC-100 DF native
		PixelSizeUm: envFloat("ASTRO_PIXEL_UM", 3.8), // ASI1600MM Pro

		// SPCC names MUST match Siril's spcc-database exactly (case/spacing). The ASI1600MM Pro's
		// sensor entry is "ZWO ASI1600MM" (no " Pro" — that name does not exist in the DB and makes
		// SPCC abort, silently falling back to green-only neutralization → a brown sky). For a mono
		// sensor SPCC also needs the per-channel filter names; default to ZWO's CMOS-optimized LRGB.
		SpccMonoSensor:      env("ASTRO_SPCC_SENSOR", "ZWO ASI1600MM"),
		SpccRFilter:         env("ASTRO_SPCC_RFILTER", "ZWO Optimized for CMOS Red"),
		SpccGFilter:         env("ASTRO_SPCC_GFILTER", "ZWO Optimized for CMOS Green"),
		SpccBFilter:         env("ASTRO_SPCC_BFILTER", "ZWO Optimized for CMOS Blue"),
		SpccWhiteRef:        env("ASTRO_SPCC_WHITEREF", "Average Spiral Galaxy"),
		NightscapeOSCSensor: env("ASTRO_NIGHTSCAPE_OSC_SENSOR", ""),
		PlateSolveCatalog:   env("ASTRO_PLATESOLVE_CATALOG", ""),
		SirilCatalogDir:     catalogDir,
		GaiaAstroCat:        env("ASTRO_GAIA_ASTRO_CAT", filepath.Join(libraryDir, "catalogues", "siril_cat_healpix8_astro.dat")),
		GaiaXpsampDir:       env("ASTRO_GAIA_XPSAMP_DIR", filepath.Join(libraryDir, "catalogues")),
		DeepStarCat:         env("ASTRO_DEEPSTAR_CAT", filepath.Join(libraryDir, "catalogues", "athyg_v32.bin")),
		LocalAsnet:          envBool("ASTRO_LOCAL_ASNET", false),
		SpccCatalog:         env("ASTRO_SPCC_CATALOG", ""),

		LatDeg:     envFloat("ASTRO_LAT", 48.8566), // Paris by default; overridable in the UI
		LonDeg:     envFloat("ASTRO_LON", 2.3522),
		Timezone:   env("ASTRO_TIMEZONE", "Europe/Paris"),
		ApertureMM: envFloat("ASTRO_APERTURE_MM", 100), // Takahashi FC-100 DF
		SensorWpx:  envInt("ASTRO_SENSOR_W", 4656),     // ASI1600MM Pro
		SensorHpx:  envInt("ASTRO_SENSOR_H", 3520),
		// A sane visual kit for the 740 mm f/7.4 FC-100 (exit pupils 4.1 → 0.8 mm).
		EyepieceKit: env("ASTRO_EYEPIECES", "30:68:30mm,18:65:18mm,10:60:10mm,6:60:6mm"),
		BarlowX:     envFloat("ASTRO_BARLOW", 1),
		ReducerX:    envFloat("ASTRO_REDUCER", 1),

		ReuseEnabled:         envBool("ASTRO_REUSE_ENABLED", true),
		ReuseConeDeg:         envFloat("ASTRO_REUSE_CONE_DEG", 0.5),
		ReuseDarkRecencyDays: envInt("ASTRO_REUSE_DARK_RECENCY_DAYS", 0),
		ReuseTempTolC:        envFloat("ASTRO_REUSE_TEMP_TOL_C", 5.0),
	}
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envFloatList parses a comma-separated list of floats (e.g. "1000,2500"), falling back to def when the
// var is unset, empty, or malformed.
// envStrList splits key on ":" or "," into a trimmed, non-empty string slice (nil when unset) — used for
// path lists like ASTRO_BROWSE_ROOTS.
func envStrList(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ':' || r == ',' }) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envFloatList(key string, def []float64) []float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var out []float64
	for _, part := range strings.Split(v, ",") {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return def
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// DarkSinceMs is the epoch-ms cutoff below which darks are too old to reuse, or 0 (unbounded) when
// ReuseDarkRecencyDays is 0.
func (c *Config) DarkSinceMs() int64 {
	if c.ReuseDarkRecencyDays <= 0 {
		return 0
	}
	return time.Now().AddDate(0, 0, -c.ReuseDarkRecencyDays).UnixMilli()
}

// Location resolves the configured observing timezone, falling back to UTC if it cannot be loaded.
func (c *Config) Location() *time.Location {
	if loc, err := time.LoadLocation(c.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// firstOnPath returns the first candidate executable resolvable on PATH — the default for a tool
// whose binary is named differently across generations. With none installed it returns the last
// candidate, so the eventual "not found" error still names a real tool.
func firstOnPath(candidates []string) string {
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return candidates[len(candidates)-1]
}

// LocalGaiaAstroCat returns the local Gaia astrometric catalogue path when the file is actually
// present, else "" — callers wire it into siril.SolveOptions only when solving can really use it
// (download once with `just download-catalogues`).
func (c *Config) LocalGaiaAstroCat() string {
	if c.GaiaAstroCat == "" {
		return ""
	}
	if st, err := os.Stat(c.GaiaAstroCat); err != nil || st.IsDir() {
		return ""
	}
	return c.GaiaAstroCat
}

// LocalGaiaXpsampDir returns the xp_sampled chunk directory when it holds at least one chunk file
// (`just download-catalogues-spcc`), else "". Siril scans the directory itself, so a partial chunk
// set covering only the shot sky regions is fine.
func (c *Config) LocalGaiaXpsampDir() string {
	if c.GaiaXpsampDir == "" {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(c.GaiaXpsampDir, "siril_cat*_xpsamp_*.dat"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return c.GaiaXpsampDir
}
