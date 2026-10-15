package telemetry

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func minimumOTelProfile(t *testing.T) bool {
	t.Helper()
	switch profile := os.Getenv("TELEMETRY_OTEL_TEST_PROFILE"); profile {
	case "", "selected":
		return false
	case "minimum-common":
		return true
	default:
		t.Fatalf("unknown OTel test profile %q", profile)
		return false
	}
}

func TestOTelVersionQualification(t *testing.T) {
	script, err := filepath.Abs("scripts/test-otel-version.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name    string
		wantErr bool
	}{
		{name: "minimum"},
		{name: "uplift", wantErr: true},
		{name: "replacement", wantErr: true},
		{name: "test-failure", wantErr: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			original := "module example.com/qualification\n\ngo 1.27.0\n"
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			fake := `#!/bin/sh
set -eu
case "$1 $2" in
  'env GOVERSION') printf 'go1.27.2\n' ;;
  'mod edit'|'mod tidy')
    modfile=go.mod
    for argument in "$@"; do
      case "$argument" in -modfile=*) modfile=${argument#-modfile=} ;; esac
    done
    printf '\n// fixture dependency edit\n' >> "$modfile"
    ;;
  'list '* )
    for module in "$@"; do
      case "$module" in
        go.opentelemetry.io/otel*)
          version=v1.46.0
          replacement=direct
          if [ "$FAKE_MODE" = uplift ] && [ "$module" = go.opentelemetry.io/otel ]; then version=v1.47.0; fi
          if [ "$FAKE_MODE" = replacement ] && [ "$module" = go.opentelemetry.io/otel ]; then replacement=replaced; fi
          printf '%s %s %s\n' "$module" "$version" "$replacement"
          ;;
      esac
    done
    ;;
  'test '* )
    printf '%s\n' "${TELEMETRY_OTEL_TEST_PROFILE:-selected}" > "$FAKE_CAPTURE"
    if [ "$FAKE_MODE" = test-failure ]; then exit 7; fi
    ;;
  *) exit 9 ;;
esac
`
			// #nosec G306 -- owner-only execute permission is required for this private fixture.
			if err := os.WriteFile(filepath.Join(bin, "go"), []byte(fake), 0o700); err != nil {
				t.Fatal(err)
			}
			capture := filepath.Join(root, "test-profile")
			// #nosec G204 -- runs the fixed repository script without shell interpolation.
			command := exec.CommandContext(t.Context(), "sh", script, "v1.46.0")
			command.Dir = root
			command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"TMPDIR="+root, "FAKE_MODE="+scenario.name, "FAKE_CAPTURE="+capture)
			output, err := command.CombinedOutput()
			if (err != nil) != scenario.wantErr {
				t.Fatalf("qualification error = %v, want error %v\n%s", err, scenario.wantErr, output)
			}
			// #nosec G304 -- reads a fixed file in this test-owned private temporary directory.
			module, err := os.ReadFile(filepath.Join(root, "go.mod"))
			if err != nil || string(module) != original {
				t.Fatalf("caller module changed: %v\n%s", err, module)
			}
			// #nosec G304 -- capture is a fixed file in this test-owned private temporary directory.
			profile, err := os.ReadFile(capture)
			if scenario.name == "uplift" || scenario.name == "replacement" {
				if !os.IsNotExist(err) {
					t.Fatal("invalid dependency graph reached tests")
				}
			} else if err != nil || strings.TrimSpace(string(profile)) != "minimum-common" {
				t.Fatalf("test profile = %q, error %v", profile, err)
			}
		})
	}
}
