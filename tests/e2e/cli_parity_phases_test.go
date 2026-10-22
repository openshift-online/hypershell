package e2e

import "testing"

func TestUIAndSuiteInstallerVersionAgree(t *testing.T) {
	for _, tt := range []struct {
		name           string
		gatewayVersion string
		want           string
		wantErr        bool
	}{
		{name: "plain semver without v", gatewayVersion: "0.1.3", want: "v0.1.3"},
		{name: "plain semver with v", gatewayVersion: "v0.1.3", want: "v0.1.3"},
		{name: "downstream suffix without v", gatewayVersion: "0.1.3-rhaiv.3", want: "v0.1.3-rhaiv.3"},
		{name: "downstream suffix with v", gatewayVersion: "v0.1.3-rhaiv.3", want: "v0.1.3-rhaiv.3"},
		{name: "surrounding whitespace", gatewayVersion: "  0.1.3  ", want: "v0.1.3"},
		{name: "empty", gatewayVersion: "", wantErr: true},
		{name: "whitespace only", gatewayVersion: "   ", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			uiGot, uiErr := uiInstallerVersion(tt.gatewayVersion)
			suiteGot, suiteErr := suiteInstallerVersion(tt.gatewayVersion)
			if tt.wantErr {
				if uiErr == nil || suiteErr == nil {
					t.Fatalf("uiInstallerVersion err=%v, suiteInstallerVersion err=%v, want both non-nil", uiErr, suiteErr)
				}
				return
			}
			if uiErr != nil || suiteErr != nil {
				t.Fatalf("uiInstallerVersion err=%v, suiteInstallerVersion err=%v, want both nil", uiErr, suiteErr)
			}
			if uiGot != tt.want {
				t.Errorf("uiInstallerVersion(%q) = %q, want %q", tt.gatewayVersion, uiGot, tt.want)
			}
			if suiteGot != tt.want {
				t.Errorf("suiteInstallerVersion(%q) = %q, want %q", tt.gatewayVersion, suiteGot, tt.want)
			}
			if uiGot != suiteGot {
				t.Errorf("UI and suite installer versions diverge: %q != %q", uiGot, suiteGot)
			}
		})
	}
}

func TestBuildOpenShellInstallCommandMatchesUIRendering(t *testing.T) {
	got := buildOpenShellInstallCommand("v0.1.3-rhaiv.3")
	want := "curl -LsSf \\\n" +
		"  " + installScriptURL + " \\\n" +
		"  | OPENSHELL_VERSION=v0.1.3-rhaiv.3 sh\n" +
		`export PATH="$HOME/.local/bin:$PATH"`
	if got != want {
		t.Errorf("buildOpenShellInstallCommand mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestSuffixedVersionRE(t *testing.T) {
	for _, tt := range []struct {
		version string
		want    bool
	}{
		{"v0.1.3", false},
		{"v0.1.3-rhaiv.3", true},
		{"v1.2.3-anything", true},
		{"0.1.3-rhaiv.3", false}, // install-openshell.sh's own case pattern requires the leading v
	} {
		if got := suffixedVersionRE.MatchString(tt.version); got != tt.want {
			t.Errorf("suffixedVersionRE.MatchString(%q) = %t, want %t", tt.version, got, tt.want)
		}
	}
}

func TestOpenshellCLIMatchesVersion(t *testing.T) {
	for _, tt := range []struct {
		name            string
		reportedVersion string
		gatewayVersion  string
		want            bool
	}{
		{name: "exact match no v", reportedVersion: "openshell 0.1.3", gatewayVersion: "0.1.3", want: true},
		{name: "v prefix on reported only", reportedVersion: "openshell v0.1.3", gatewayVersion: "0.1.3", want: true},
		{name: "v prefix on gateway only", reportedVersion: "openshell 0.1.3", gatewayVersion: "v0.1.3", want: true},
		{name: "suffix matches", reportedVersion: "openshell 0.1.3-rhaiv.3", gatewayVersion: "v0.1.3-rhaiv.3", want: true},
		{name: "mismatch", reportedVersion: "openshell 0.1.2", gatewayVersion: "0.1.3", want: false},
		{name: "wrong binary name", reportedVersion: "openshel 0.1.3", gatewayVersion: "0.1.3", want: false},
		{name: "unparseable output", reportedVersion: "garbage", gatewayVersion: "0.1.3", want: false},
		{name: "trailing tokens ignored", reportedVersion: "openshell 0.1.3 (build abc)", gatewayVersion: "0.1.3", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := openshellCLIMatchesVersion(tt.reportedVersion, tt.gatewayVersion); got != tt.want {
				t.Errorf("openshellCLIMatchesVersion(%q, %q) = %t, want %t", tt.reportedVersion, tt.gatewayVersion, got, tt.want)
			}
		})
	}
}

func TestShellArgument(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  string
	}{
		{"v0.1.3-rhaiv.3", "v0.1.3-rhaiv.3"},
		{"simple", "simple"},
		{"has space", `'has space'`},
		{"it's got a quote", `'it'"'"'s got a quote'`},
	} {
		if got := shellArgument(tt.value); got != tt.want {
			t.Errorf("shellArgument(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}
