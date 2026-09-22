package preset

// Builtins is the curated "best params per situation" catalog. Each entry is a starting point the user can
// apply with one click and then fine-tune. Slugs are stable (they key the i18n labels + the anti-drift
// test); the numeric recipes are deltas on top of each mode's default preset (mode.For), expressed with
// the same knob keys the Advanced box and the supervisor use, so they can never fall out of range without
// the catalog test catching it. supervise stays off everywhere — it is slow and needs the local VLM, so
// it is opt-in per run, not baked into a preset.
func Builtins() []Item {
	out := make([]Item, 0, 16)
	out = append(out, deepskyBuiltins()...)
	out = append(out, stackingBuiltins()...)
	out = append(out, nebulaBuiltins()...)
	out = append(out, narrowbandBuiltins()...)
	out = append(out, solarBuiltins()...)
	out = append(out, sunBuiltins()...)
	out = append(out, cometBuiltins()...)
	out = append(out, milkywayBuiltins()...)
	return out
}

// deepskyBuiltins — broadband deep-sky targets (galaxies, clusters, reflection nebulae): mode deepsky,
// natural palette, colour calibration + denoise on.
func deepskyBuiltins() []Item {
	return []Item{
		builtin("galaxy-lrgb", CategoryDeepsky, objects(ObjectGalaxy), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// Bright core + faint outer arms: a touch more highlight headroom so the core keeps colour,
			// gentle star reduction, moderate luminance denoise for the faint disk. Saturation 0.20:
			// with SPCC actually running (task #316 fix) the balance is photometric, and galaxies need
			// the extra chroma to show their blue-arm/orange-core contrast instead of grey.
			Params: mustParams(map[string]any{
				"stretch_headroom": 0.92, "star_reduce": 0.1, "denoise_lum": 0.4, "saturation": 0.20,
			}),
		}),
		builtin("galaxy-faint", CategoryDeepsky, objects(ObjectGalaxy), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// Low-SNR faint galaxy: stronger denoise, AI background extraction on the combined masters,
			// a little more star reduction to keep the faint structure readable.
			Params: mustParams(map[string]any{
				"denoise_lum": 0.65, "denoise_chroma": 0.9, "background_ai": true,
				"combined_background_ai": true, "star_reduce": 0.15, "stretch_headroom": 0.88,
			}),
		}),
		builtin("star-cluster", CategoryDeepsky, objects(ObjectStarCluster), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// Globular / open cluster: never reduce stars; tame the colour discs/mottle around bright
			// stars (star desaturation + chroma blur) while keeping full luminance — the cluster-finish fix.
			Params: mustParams(map[string]any{
				"star_reduce": 0, "star_desat": 0.6, "chroma_blur": 4, "lum_opacity": 1.0,
			}),
		}),
		builtin("reflection-nebula", CategoryDeepsky, objects(ObjectReflectionNebula), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// Blue broadband reflection nebula: no Ha screen, a little more saturation to hold the blues,
			// gentle star reduction.
			Params: mustParams(map[string]any{
				"ha_screen": 0, "saturation": 0.16, "denoise_lum": 0.45, "star_reduce": 0.1,
			}),
		}),
		builtin("dark-nebula", CategoryDeepsky, objects(ObjectDarkNebula), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// LDN/Barnard dark nebulae are the LOWEST-stretch case in the whole catalog: the subject is
			// an absence of light, and every point of extra stretch fills the lane back in with sky. So
			// background_level sits at the floor (0.05), no emission screen runs, and the faint dust is
			// carried by denoise rather than by lifting the black point. Rejection is left untouched
			// (winsorized) on purpose — see stacking-doc §8.3.
			Params: mustParams(map[string]any{
				"background_level": 0.05, "ha_screen": 0, "star_reduce": 0.05,
				"denoise_lum": 0.6, "denoise_chroma": 0.7, "saturation": 0.12, "stretch_headroom": 0.9,
			}),
		}),
		builtin("osc-broadband", CategoryDeepsky, objects(ObjectGalaxy, ObjectEmissionNebula, ObjectReflectionNebula, ObjectStarCluster), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// A THIN one-shot-colour broadband night, shot for star colour rather than for depth. The
			// deepsky default runs four chroma-reducing stages (AI colour denoise + chroma_smooth 6 +
			// chroma_bg_smooth 24 + sky-chroma flattening); on a thin stack that is measured to HALVE
			// the chroma (0.138 → 0.073), so all four are off here and the colour is defended instead
			// by saturation at its ceiling. roundness_floor 0.66 keeps only well-guided subs (the 0.55
			// default is ~1.8:1 elongation). Runbook: ngc7000-narrowband-plus-broadband §3.
			//
			// saturation is 0.35, not the runbook's 0.50: the deepsky clamp caps it at 0.35, so 0.50
			// silently becomes this. The catalog states what the engine actually applies.
			Params: mustParams(map[string]any{
				"crop_frac": 0.03, "background_level": 0.08, "star_reduce": 0,
				"roundness_floor": 0.66, "color_denoise_ai": false,
				"chroma_smooth_px": 0, "chroma_bg_smooth_px": 0, "sky_chroma_flatten_px": 0,
				"denoise_chroma": 0.35, "saturation": 0.35, "star_desat": 0,
			}),
		}),
		builtin("wide-field-lens", CategoryDeepsky, objects(ObjectMilkyway, ObjectEmissionNebula, ObjectDarkNebula), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			Denoise: boolPtr(true),
			// A camera LENS rather than a telescope (measured on a 20 mm f/1.8 OSC series, 151×60 s).
			// At that focal length the frame is mostly sky, and the two AI stages read the sky itself as
			// structure: the background extractor clumps, and the colour denoiser leaves a magenta cast.
			// Both off, both sky-flattening passes off, and the gradient modelled as a simple plane
			// (background_degree 1) — a higher-degree surface chases the Milky Way's own light.
			//
			// color_calibration is deliberately left at the form default: a field this wide often
			// exceeds the plate solver's usable domain, so the run may fall back rather than solve.
			Params: mustParams(map[string]any{
				"color_denoise_ai": false, "background_ai": false,
				"sky_chroma_flatten_px": 0, "sky_lum_flatten_px": 0, "background_degree": 1,
			}),
		}),
	}
}

// stackingBuiltins — recipes that change HOW the frames are combined rather than how the result is
// graded. They are the situations where the count-adaptive default is not the best answer; every
// other knob is left at the mode default so the effect of the stacking choice is isolated. See
// docs/stacking.md.
func stackingBuiltins() []Item {
	return []Item{
		builtin("stack-moonlit-gradient", CategoryDeepsky, objects(ObjectGalaxy, ObjectEmissionNebula, ObjectReflectionNebula, ObjectDarkNebula, ObjectStarCluster), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// A sky that MOVED during the session (rising moon, drifting light pollution): linear-fit
			// clipping models the changing level per pixel instead of treating it as noise, which a
			// sigma-based test cannot do. The asymmetric sigmas protect the faint end.
			Params: mustParams(map[string]any{
				"stack_reject": "linear_fit", "stack_reject_low": 5, "stack_reject_high": 3.5,
			}),
		}),
		builtin("stack-deep-session", CategoryDeepsky, objects(ObjectGalaxy, ObjectEmissionNebula, ObjectPlanetaryNebula, ObjectStarCluster), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// A long session (50+ subs) where the correlated leftovers matter more than raw depth:
			// GESD with a tighter significance rejects the walking-noise and trail remnants a fixed
			// 3σ clip leaves behind, and noise weighting favours the cleanest subs.
			Params: mustParams(map[string]any{
				"stack_reject": "gesd", "stack_reject_low": 0.3, "stack_reject_high": 0.02,
				"stack_weight": "noise",
			}),
		}),
		builtin("stack-few-frames", CategoryDeepsky, objects(ObjectGalaxy, ObjectEmissionNebula, ObjectReflectionNebula, ObjectStarCluster), Payload{
			Mode: "deepsky", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// A handful of dissimilar subs: no measured sigma is trustworthy, so clip a fixed share at
			// each end and keep the weighting off — with so few frames, weighting one of them down
			// costs more depth than the sharpness buys.
			Params: mustParams(map[string]any{
				"stack_reject": "percentile", "stack_reject_low": 0.2, "stack_reject_high": 0.1,
				"stack_weight": "none",
			}),
		}),
	}
}

// nebulaBuiltins — broadband + Ha emission nebulae: mode nebula (Ha-forward), colour calibration on.
func nebulaBuiltins() []Item {
	return []Item{
		builtin("emission-hargb", CategoryNebula, objects(ObjectEmissionNebula), Payload{
			Mode: "nebula", Format: "image", Palette: "hargb",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true), HaExcludeStars: boolPtr(true),
			// Ha blended into RGB: a strong Ha screen with a raised black point to keep the background
			// clean, moderate star reduction so the nebula reads over the star field.
			Params: mustParams(map[string]any{
				"ha_screen": 0.55, "ha_black_point": 0.12, "star_reduce": 0.4, "saturation": 0.15,
			}),
		}),
		builtin("emission-broadband", CategoryNebula, objects(ObjectEmissionNebula), Payload{
			Mode: "nebula", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true), HaExcludeStars: boolPtr(true),
			// Broadband emission nebula (no dedicated Ha stack): a mild Ha screen, light star reduction.
			Params: mustParams(map[string]any{
				"ha_screen": 0.35, "star_reduce": 0.3, "saturation": 0.14,
			}),
		}),
		builtin("planetary-nebula", CategoryNebula, objects(ObjectPlanetaryNebula), Payload{
			Mode: "nebula", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// Small, bright planetary nebula: keep every star, extra saturation for the shell colours, a
			// gentler stretch so the bright core does not blow out.
			Params: mustParams(map[string]any{
				"star_reduce": 0, "saturation": 0.18, "stretch_headroom": 0.9, "ha_screen": 0.45,
			}),
		}),
		builtin("oxygen-cloud", CategoryNebula, objects(ObjectOxygenCloud), Payload{
			Mode: "nebula", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// An [OIII]-dominant object (OU4's squid, big OIII shells). OIII is the faintest and the most
			// light-pollution-sensitive channel there is, so sky CLEANLINESS beats PSF sharpness when
			// weighting frames — stack_weight noise rather than the usual wfwhm. The teal shell is put
			// on top by the OIII screen instead of by stretch, star reduction lets it read over the star
			// field, and the remaining noise is denoised away rather than stretched. stacking-doc §8.2.
			// With Ha frames in hand, prefer the narrowband-hoo recipe — it owns its own colour.
			Params: mustParams(map[string]any{
				"oiii_screen": 0.5, "star_reduce": 0.45, "star_desat": 0.3,
				"denoise_lum": 0.6, "background_level": 0.075, "stack_weight": "noise",
			}),
		}),
		builtin("supernova-remnant", CategoryNebula, objects(ObjectSupernovaRemnant), Payload{
			Mode: "nebula", Format: "image", Palette: "natural",
			ColorCalibration: boolPtr(true), Denoise: boolPtr(true),
			// Veil-class remnant: filaments in BOTH Ha and OIII, so both screens run — the structure is
			// the interplay between them. These are wide fields, which means gradients, so linear-fit
			// rejection models the changing sky level per pixel instead of treating it as noise. Heavy
			// star reduction (0.5) is what makes the filaments readable at all. stacking-doc §8.3.
			Params: mustParams(map[string]any{
				"ha_screen": 0.45, "oiii_screen": 0.45, "star_reduce": 0.5, "star_desat": 0.35,
				"saturation":   0.17,
				"stack_reject": "linear_fit", "stack_reject_low": 5, "stack_reject_high": 3.5,
			}),
		}),
	}
}

// narrowbandBuiltins — palette-mapped narrowband (SHO/HOO/Foraxx): mode nebula with SPCC OFF (narrowband
// palettes do their own colour mapping) and the magenta star haloes tamed.
func narrowbandBuiltins() []Item {
	return []Item{
		builtin("narrowband-sho", CategoryNarrowband, objects(ObjectEmissionNebula, ObjectSupernovaRemnant), Payload{
			Mode: "nebula", Format: "image", Palette: "sho",
			ColorCalibration: boolPtr(false), Denoise: boolPtr(true), HaExcludeStars: boolPtr(true),
			Params: mustParams(map[string]any{
				"star_reduce": 0.5, "star_desat": 0.4, "saturation": 0.16,
			}),
		}),
		builtin("narrowband-hoo", CategoryNarrowband, objects(ObjectEmissionNebula, ObjectOxygenCloud, ObjectSupernovaRemnant), Payload{
			Mode: "nebula", Format: "image", Palette: "hoo",
			ColorCalibration: boolPtr(false), Denoise: boolPtr(true), HaExcludeStars: boolPtr(true),
			Params: mustParams(map[string]any{
				"star_reduce": 0.4, "star_desat": 0.3, "saturation": 0.16,
			}),
		}),
		builtin("narrowband-foraxx", CategoryNarrowband, objects(ObjectEmissionNebula, ObjectSupernovaRemnant), Payload{
			Mode: "nebula", Format: "image", Palette: "foraxx",
			ColorCalibration: boolPtr(false), Denoise: boolPtr(true), HaExcludeStars: boolPtr(true),
			Params: mustParams(map[string]any{
				"star_reduce": 0.5, "star_desat": 0.35, "saturation": 0.17,
			}),
		}),
	}
}

// solarBuiltins — Moon & planets via lucky imaging: mode planetary.
func solarBuiltins() []Item {
	return []Item{
		builtin("moon", CategorySolar, objects(ObjectMoon), Payload{
			Mode: "planetary", Format: "image",
			// Bright full disk: reserve highlight headroom so it does not burn. True lucky-imaging
			// selection: the 2026-07-12 run forensics showed a bigger kept fraction only dilutes the
			// sharp minority (65%→30% already measurably sharpened the master; 15 is the package
			// default the per-AP selection is tuned for). The old sharpen 1.2 / clahe 1.5 push
			// compensated the pre-canonical soft master; on the two-level/canonical/drizzle stack the
			// package finish defaults render clean detail without it (2026-07-14 real-run crops) —
			// the extra push only added the harsh, haloed look.
			// shadow_lift 0.35 opens the crushed dark maria/terminator tones (the user preferred the
			// "less-dark, more detail, more natural" render); saturation 0.4 reveals the grey/brown
			// mineral tones naturally (still far below the garish 0.8 default). Both are 2026-07 A/B
			// starting points — validated against real 100% crops before shipping.
			// limb_balance 0.55 compresses the smooth illumination of the bright limb band (local
			// crater contrast untouched) — a crescent/gibbous limb no longer stretches to burnt
			// white while the terminator keeps its opened shadows.
			Params: mustParams(map[string]any{
				"headroom": 0.85, "best_percent": 15, "saturation": 0.4, "shadow_lift": 0.35,
				"limb_balance": 0.55,
			}),
		}),
		builtin("planet", CategorySolar, objects(ObjectPlanet), Payload{
			Mode: "planetary", Format: "both",
			// Small bright disk: strict frame selection, alignment-point warping, a mild luminance
			// deconvolution and a saturation boost for the real disc colour.
			Params: mustParams(map[string]any{
				"best_percent": 10, "ap_align": true, "deconv_fwhm": 3, "deconv_iters": 20,
				"deconv_alpha": 1500, "sharpen": 1.4, "clahe": 1.0, "saturation": 0.8,
			}),
		}),
	}
}

// cometBuiltins — moving comet: mode comet (dual star/comet stack + star-layer recomposite).
func cometBuiltins() []Item {
	return []Item{
		builtin("comet", CategoryComet, objects(ObjectComet), Payload{
			Mode: "comet", Format: "image",
			// A gentle coma-gradient removal and a touch of saturation for the ion/dust tail colour; the
			// dual star/comet stacking is handled by the mode itself.
			Params: mustParams(map[string]any{
				"background_degree": 2, "saturation": 0.14,
			}),
		}),
	}
}

// milkywayBuiltins — wide-field nightscapes: mode milkyway. The recipe is the look + brightness (the
// nightscape renderer owns the rest); no Advanced knobs.
func milkywayBuiltins() []Item {
	return []Item{
		builtin("milkyway-natural", CategoryMilkyway, objects(ObjectMilkyway), Payload{
			Mode: "milkyway", Format: "image", Look: "natural", Brightness: "balanced",
		}),
		builtin("milkyway-iphone", CategoryMilkyway, objects(ObjectMilkyway), Payload{
			Mode: "milkyway", Format: "image", Look: "iphone", Brightness: "balanced",
		}),
		builtin("milkyway-deep", CategoryMilkyway, objects(ObjectMilkyway, ObjectDarkNebula), Payload{
			Mode: "milkyway", Format: "image", Look: "deepsky", Brightness: "brighter",
		}),
	}
}

// sunBuiltins — the Sun in Hα or white light: mode sun.
//
// NONE of these names a deconvolution width, and that is deliberate rather than an omission. The
// width is the one setting here with a true value rather than a tasteful one — it is the measured
// point spread function — and naming it is precisely how a user turns the measurement OFF
// (params_sun.go). Every one of these presets used to pin it at 1.2–1.5 px, tuned when that was the
// only number available, against captures that measure three to four. So each preset shipped a
// deliberately under-deconvolved rendering and none of them could ever benefit from the measurement.
// The iteration counts went the same way: they were companions to that narrow width.
//
// What is left here is the part that really is taste — how hard to flatten the limb, how far to push
// the prominences, which palette, how much local contrast.
func sunBuiltins() []Item {
	return []Item{
		builtin("sun_ha_full", CategorySun, objects(ObjectSun), Payload{
			Mode: "sun", Format: "both",
			// The default: disc detail and prominences in one frame, which is what an Hα scope is for.
			Params: mustParams(map[string]any{
				"limb_flatten": 0.85, "prominence_boost": 1.0,
				"sharpen_medium": 1.35, "palette": "gold",
			}),
		}),
		builtin("sun_ha_disk", CategorySun, objects(ObjectSun), Payload{
			Mode: "sun", Format: "image",
			// Chromosphere detail only: the limb is flattened hard and the prominences are pulled back
			// so nothing competes with filaments and plage across the surface.
			Params: mustParams(map[string]any{
				"limb_flatten": 1.0, "prominence_boost": 0.2,
				"sharpen_small": 1.3, "sharpen_medium": 1.6, "palette": "gold", "contrast": 1.15,
			}),
		}),
		builtin("sun_ha_prominence", CategorySun, objects(ObjectSun), Payload{
			Mode: "sun", Format: "image",
			// Limb-forward: the off-limb material is stretched hard and the disc is left dark, the way
			// a prominence close-up is normally presented.
			Params: mustParams(map[string]any{
				"limb_flatten": 0.3, "prominence_boost": 2.5, "prominence_feather": 0.012,
				"stretch": 0.7, "palette": "gold",
			}),
		}),
		builtin("sun_whitelight", CategorySun, objects(ObjectSun), Payload{
			Mode: "sun", Format: "image",
			// Photosphere through a solar film: sunspots and faculae, neutral rendering, no prominences
			// to show (a white-light filter passes none).
			Params: mustParams(map[string]any{
				"band": "white_light", "limb_flatten": 0.6, "prominence_boost": 0,
				"palette": "neutral", "saturation": 0.6,
			}),
		}),
		builtin("sun_iphone_video", CategorySun, objects(ObjectSun), Payload{
			Mode: "sun", Format: "both",
			// A phone clip through the eyepiece. The starlet gains are pulled back because the camera
			// pipeline has already sharpened once, and double-sharpening haloes every limb and filament.
			// The DECONVOLUTION half of that concern is now measured rather than assumed: the run reads
			// the phone's own overshoot off the limb and cuts the iteration count in proportion, which
			// is both more accurate than a fixed cut and survives the phone, the app or the firmware
			// changing (autotune.go).
			Params: mustParams(map[string]any{
				"keep_percent": 50, "max_frames": 400, "window_seconds": 30,
				"sharpen_small": 0.9, "sharpen_medium": 1.15, "sharpen_denoise": 0.5, "palette": "gold",
			}),
		}),
	}
}
