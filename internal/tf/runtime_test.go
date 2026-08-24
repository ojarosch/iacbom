package tf

import "testing"

func TestDetectPins(t *testing.T) {
	contents := map[string]string{
		".terraform-version": "1.13.1\n",
		".opentofu-version":  "1.11.2\n",
		".tool-versions":     "nodejs 20.0.0\nterraform 1.9.8\nopentofu 1.8.0\n",
		"mise.toml":          "[tools]\nopentofu = \"1.11.0\"\npython = \"3.12\"\n",
	}
	pins := DetectPins(contents)
	got := map[string]string{}
	for _, p := range pins {
		if _, dup := got[p.Runtime]; !dup {
			got[p.Runtime] = p.Version // first pin wins
		}
	}
	want := map[string]string{
		"terraform": "1.13.1",
		"opentofu":  "1.11.2",
	}
	for rt, v := range want {
		if got[rt] != v {
			t.Errorf("runtime %s pinned to %q, want %q", rt, got[rt], v)
		}
	}
}

func TestDetectPinsEmpty(t *testing.T) {
	if pins := DetectPins(map[string]string{}); len(pins) != 0 {
		t.Errorf("expected no pins, got %v", pins)
	}
	if pins := DetectPins(map[string]string{".terraform-version": "\n \n"}); len(pins) != 0 {
		t.Errorf("blank version file should produce no pin, got %v", pins)
	}
}
