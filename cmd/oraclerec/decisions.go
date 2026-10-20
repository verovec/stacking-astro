package main

import (
	"strings"

	"github.com/verove-jordan/astronomy/internal/calib"
	"github.com/verove-jordan/astronomy/internal/grade"
	"github.com/verove-jordan/astronomy/internal/pipeline"
)

// The decision files: views of the run Result (what the run actually did, not a separate plan)
// plus the commands that only exist in the captured scripts.

type channelKey struct {
	Object string `json:"object"`
	Filter string `json:"filter"`
}

func keyOf(ch pipeline.ChannelResult) channelKey { return channelKey{ch.Object, ch.Filter} }

type groupCalibration struct {
	Session    string `json:"session,omitempty"`
	Filter     string `json:"filter"`
	Frames     int    `json:"frames"`
	Dark       string `json:"dark,omitempty"`
	Flat       string `json:"flat,omitempty"`
	Bias       string `json:"bias,omitempty"`
	FlatSource string `json:"flat_source,omitempty"`
}

type channelCalibration struct {
	channelKey
	Selection calib.Selection    `json:"selection"`
	Groups    []groupCalibration `json:"groups,omitempty"`
}

type calibrationFile struct {
	Masters  []calib.Master       `json:"masters"`
	Channels []channelCalibration `json:"channels"`
}

func calibrationOf(res *pipeline.Result) calibrationFile {
	file := calibrationFile{Masters: res.Masters, Channels: []channelCalibration{}}
	for _, ch := range res.Channels {
		cc := channelCalibration{channelKey: keyOf(ch), Selection: ch.Selection}
		for _, g := range ch.Groups {
			cc.Groups = append(cc.Groups, groupCalibration{Session: g.Session, Filter: g.Filter,
				Frames: g.Frames, Dark: g.Dark, Flat: g.Flat, Bias: g.Bias, FlatSource: g.FlatSource})
		}
		file.Channels = append(file.Channels, cc)
	}
	return file
}

type channelGrades struct {
	channelKey
	Metrics []grade.Metric `json:"metrics"`
}

func gradesOf(res *pipeline.Result) []channelGrades {
	out := []channelGrades{}
	for _, ch := range res.Channels {
		metrics := ch.Metrics
		if metrics == nil {
			metrics = []grade.Metric{}
		}
		out = append(out, channelGrades{channelKey: keyOf(ch), Metrics: metrics})
	}
	return out
}

type rejectedFrame struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type groupCount struct {
	Session        string `json:"session,omitempty"`
	StackedFrames  int    `json:"stacked_frames"`
	RejectedFrames int    `json:"rejected_frames"`
}

type channelSelection struct {
	channelKey
	Synthesized   bool            `json:"synthesized,omitempty"`
	InputFrames   int             `json:"input_frames"`
	StackedFrames int             `json:"stacked_frames"`
	Kept          []string        `json:"kept"`
	Rejected      []rejectedFrame `json:"rejected"`
	Groups        []groupCount    `json:"groups,omitempty"`
}

// scriptCommand is one command line found in a captured script.
type scriptCommand struct {
	Script  string `json:"script"`
	Command string `json:"command"`
}

type selectionFile struct {
	Channels []channelSelection `json:"channels"`
	// Unselect lists every `unselect` the stack scripts ran: the registered-space frames dropped,
	// which run.json does not carry.
	Unselect []scriptCommand `json:"unselect"`
}

func selectionOf(res *pipeline.Result, scripts []recordedScript) selectionFile {
	file := selectionFile{Channels: []channelSelection{}, Unselect: commands(scripts, "unselect ")}
	for _, ch := range res.Channels {
		cs := channelSelection{channelKey: keyOf(ch), Synthesized: ch.Synthesized,
			InputFrames: ch.InputFrames, StackedFrames: ch.StackedFrames,
			Kept: []string{}, Rejected: []rejectedFrame{}}
		for _, m := range ch.Metrics {
			if m.Rejected {
				cs.Rejected = append(cs.Rejected, rejectedFrame{Path: m.Path, Reason: m.RejectReason})
			} else {
				cs.Kept = append(cs.Kept, m.Path)
			}
		}
		for _, g := range ch.Groups {
			cs.Groups = append(cs.Groups, groupCount{Session: g.Session,
				StackedFrames: g.StackedFrames, RejectedFrames: g.RejectedFrames})
		}
		file.Channels = append(file.Channels, cs)
	}
	return file
}

type groupGeometry struct {
	Session       string   `json:"session,omitempty"`
	RotationDeg   *float64 `json:"rotation_deg,omitempty"`
	OverlapFrac   *float64 `json:"overlap_frac,omitempty"`
	ParityFlipped bool     `json:"parity_flipped,omitempty"`
}

type channelRegistration struct {
	channelKey
	// PreRejected are frames the registration review dropped before stacking (absurd transforms).
	PreRejected []string        `json:"pre_rejected"`
	Groups      []groupGeometry `json:"groups,omitempty"`
}

type registrationFile struct {
	AnchorNight string                `json:"anchor_night,omitempty"`
	Channels    []channelRegistration `json:"channels"`
	// Setref lists every `setref` the scripts ran: the reference frame each registration used.
	Setref []scriptCommand `json:"setref"`
}

func registrationOf(res *pipeline.Result, scripts []recordedScript) registrationFile {
	file := registrationFile{AnchorNight: res.AnchorNight, Channels: []channelRegistration{},
		Setref: commands(scripts, "setref ")}
	for _, ch := range res.Channels {
		cr := channelRegistration{channelKey: keyOf(ch), PreRejected: []string{}}
		for _, m := range ch.Metrics {
			if strings.Contains(m.RejectReason, "absurd registration") {
				cr.PreRejected = append(cr.PreRejected, m.Path)
			}
		}
		for _, g := range ch.Groups {
			cr.Groups = append(cr.Groups, groupGeometry{Session: g.Session,
				RotationDeg: g.RotationDeg, OverlapFrac: g.OverlapFrac, ParityFlipped: g.ParityFlipped})
		}
		file.Channels = append(file.Channels, cr)
	}
	return file
}

func commands(scripts []recordedScript, prefix string) []scriptCommand {
	out := []scriptCommand{}
	for _, s := range scripts {
		for _, line := range s.lines {
			if strings.HasPrefix(strings.TrimSpace(line), prefix) {
				out = append(out, scriptCommand{Script: s.name, Command: strings.TrimSpace(line)})
			}
		}
	}
	return out
}
