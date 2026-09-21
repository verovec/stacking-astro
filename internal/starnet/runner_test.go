package starnet

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveArgs(t *testing.T) {
	tests := []struct {
		name    string
		variant Variant
		opts    Options
		want    []string
	}{
		{"positional default stride omitted", VariantPositional, Options{}, []string{"in.tif", "out.tif"}},
		{"positional explicit stride appended", VariantPositional, Options{Stride: 128}, []string{"in.tif", "out.tif", "128"}},
		{"flags default stride omitted", VariantFlags, Options{}, []string{"-i", "in.tif", "-o", "out.tif"}},
		{"flags explicit stride", VariantFlags, Options{Stride: 128}, []string{"-i", "in.tif", "-o", "out.tif", "-s", "128"}},
		{"unset variant falls back to positional", Variant(""), Options{}, []string{"in.tif", "out.tif"}},
		{"unknown variant falls back to positional", Variant("nonsense"), Options{}, []string{"in.tif", "out.tif"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, removeArgs(tt.variant, "in.tif", "out.tif", tt.opts))
		})
	}
}

func TestVariantFromMachineInfo(t *testing.T) {
	const starnet2Info = `{"schema":"starnetastro.cli.machine-info.v1","product":"starnet2","version":"2.5.4"}`
	tests := []struct {
		name string
		out  string
		err  error
		want Variant
	}{
		{"starnet2 machine-info", starnet2Info, nil, VariantFlags},
		{"machine-info with leading noise", "loading weights\n" + starnet2Info, nil, VariantFlags},
		{"probe failed (old StarNet++ rejects the flag)", "", assert.AnError, VariantPositional},
		{"unparseable output", "Usage: StarNet++ in.tif out.tif 256", nil, VariantPositional},
		{"json without the starnet schema", `{"schema":"something.else.v1"}`, nil, VariantPositional},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, variantFromMachineInfo([]byte(tt.out), tt.err))
		})
	}
}

// TestResolveVariant_ExplicitWins pins that a configured variant short-circuits the probe: an
// explicit choice must never shell out (the binary may be absent or slow to start).
func TestResolveVariant_ExplicitWins(t *testing.T) {
	tests := []struct {
		name string
		set  Variant
		want Variant
	}{
		{"flags configured", VariantFlags, VariantFlags},
		{"positional configured", VariantPositional, VariantPositional},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A bin that cannot exist: reaching the probe would yield positional, so a flags
			// expectation proves the configured value short-circuited it.
			r := NewVariant("/nonexistent/starnet-binary", tt.set)
			assert.Equal(t, tt.want, r.resolveVariant(context.Background()))
		})
	}
}

func TestParsePercent(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{"plain", "Done: 75%", 75},
		{"no percent", "Loading model", -1},
		{"over 100", "weird 999%", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parsePercent(tt.line))
		})
	}
}

func TestAvailable_EmptyBin(t *testing.T) {
	assert.Error(t, New("").Available(context.Background()))
}
