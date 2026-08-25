package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Main(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := run(t, "--version")
	if code != 0 || !strings.Contains(out, "0.1.1") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestUnknownFormatIsFatal(t *testing.T) {
	code, _, errOut := run(t, "--format", "yaml", "../../testdata/terraform-basic")
	if code != 2 || !strings.Contains(errOut, "unknown format") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestMissingPathIsFatal(t *testing.T) {
	code, _, _ := run(t, "/definitely/not/a/dir")
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
}

func TestJSONOutputIsValidAndStable(t *testing.T) {
	code1, out1, _ := run(t, "--format", "json", "../../testdata/terraform-basic")
	code2, out2, _ := run(t, "--format", "json", "../../testdata/terraform-basic")
	if code1 != 0 || code2 != 0 {
		t.Fatalf("exit codes %d/%d", code1, code2)
	}
	if out1 != out2 {
		t.Error("JSON output is not deterministic")
	}
	for _, want := range []string{`"schema_version": "1"`, `"runtime"`, `"providers"`} {
		if !strings.Contains(out1, want) {
			t.Errorf("missing %s in JSON output", want)
		}
	}
}

func TestSubsetCommands(t *testing.T) {
	for _, subset := range []string{"providers", "modules", "tools"} {
		code, out, _ := run(t, subset, "../../testdata/terraform-basic")
		if code != 0 {
			t.Errorf("%s: code = %d", subset, code)
		}
		if strings.Contains(out, "Backend") || strings.Contains(out, "Runtime\n") {
			t.Errorf("%s: output should be filtered, got:\n%s", subset, out)
		}
	}
}

func TestWarningsExitCodeOne(t *testing.T) {
	code, _, _ := run(t, "../../testdata/malformed-hcl")
	if code != 1 {
		t.Errorf("code = %d, want 1 (BOM generated with warnings)", code)
	}
}

func TestDiffCommand(t *testing.T) {
	// Produce two BOMs from different fixtures, then diff them.
	code1, oldJSON, _ := run(t, "--format", "json", "../../testdata/terraform-basic")
	code2, newJSON, _ := run(t, "--format", "json", "../../testdata/providers-lockfile")
	if code1 != 0 || code2 != 0 {
		t.Fatalf("setup failed: %d/%d", code1, code2)
	}
	oldPath := writeTemp(t, "old.json", oldJSON)
	newPath := writeTemp(t, "new.json", newJSON)

	code, out, _ := run(t, "diff", oldPath, newPath)
	if code != 1 {
		t.Errorf("diff exit = %d, want 1 (changes found)", code)
	}
	if !strings.Contains(out, "~ hashicorp/aws") && !strings.Contains(out, "- ") {
		t.Errorf("unexpected diff output:\n%s", out)
	}

	// Same file twice -> no changes -> exit 0
	code, out, _ = run(t, "diff", oldPath, oldPath)
	if code != 0 || strings.Contains(out, "+") {
		t.Errorf("identical diff: code=%d out=%q", code, out)
	}
}

func TestDiffBadInputIsFatal(t *testing.T) {
	p := writeTemp(t, "bad.json", "not json")
	if code, _, _ := run(t, "diff", p); code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if code, _, errOut := run(t, "diff"); code != 2 || !strings.Contains(errOut, "two arguments") {
		t.Errorf("missing args: code=%d err=%q", code, errOut)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
