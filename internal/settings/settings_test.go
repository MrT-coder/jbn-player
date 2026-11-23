package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlatOrDisabledEqualizerAddsNoFilter(t *testing.T) {
	if f := (EQ{Enabled: true, Gains: Presets["flat"]}).Filter(); f != "" {
		t.Fatalf("flat must not add a filter, it would cost CPU for nothing: %q", f)
	}
	if f := (EQ{Enabled: false, Gains: Presets["bass"]}).Filter(); f != "" {
		t.Fatalf("a disabled equalizer must not filter: %q", f)
	}
}

func TestFilterOnlyIncludesBandsThatChange(t *testing.T) {
	e := EQ{Enabled: true, Gains: [10]float64{6, 0, 0, 0, 0, 0, 0, 0, 0, -3}}
	got := e.Filter()
	want := "lavfi=[equalizer=f=31:t=o:w=1:g=6.0,equalizer=f=16000:t=o:w=1:g=-3.0]"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestGainsAreClamped(t *testing.T) {
	e := EQ{Enabled: true, Gains: [10]float64{99, -99}}
	f := e.Filter()
	if !strings.Contains(f, "g=12.0") || !strings.Contains(f, "g=-12.0") || strings.Contains(f, "99") {
		t.Fatalf("not clamped: %s", f)
	}
}

func TestEveryPresetIsListedAndWithinLimits(t *testing.T) {
	for _, name := range PresetOrder {
		if _, ok := Presets[name]; !ok {
			t.Fatalf("preset %q listed but missing", name)
		}
	}
	if len(Presets) != len(PresetOrder) {
		t.Fatal("PresetOrder and Presets disagree")
	}
	for name, g := range Presets {
		for _, v := range g {
			if v > MaxGain || v < -MaxGain {
				t.Fatalf("preset %q exceeds the limit", name)
			}
		}
	}
}

func TestNormalizeFixesVolumeAndDevice(t *testing.T) {
	s := Settings{Volume: 250, AudioDevice: "  "}
	s.Normalize()
	if s.Volume != 100 || s.AudioDevice != AutoDevice {
		t.Fatalf("%+v", s)
	}
	s = Settings{Volume: -5}
	s.Normalize()
	if s.Volume != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestLoadMissingGivesDefaultsAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	s, err := Load(path)
	if err != nil || s.Volume != 100 || s.AudioDevice != AutoDevice || s.EQ.Enabled {
		t.Fatalf("defaults: %+v, %v", s, err)
	}
	s.Volume = 35
	s.AudioDevice = "wasapi/{abc}"
	s.EQ = EQ{Enabled: true, Preset: "bass", Gains: Presets["bass"]}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil || back != s {
		t.Fatalf("round trip: %+v vs %+v (%v)", back, s, err)
	}
}

func TestLoadCorruptFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	_ = os.WriteFile(path, []byte("{not json"), 0o600)
	s, err := Load(path)
	if err == nil {
		t.Fatal("a corrupt file must be reported")
	}
	if s.Volume != 100 {
		t.Fatalf("must still return usable defaults: %+v", s)
	}
}
