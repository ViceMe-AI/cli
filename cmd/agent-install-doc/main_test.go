package main

import (
	"os"
	"strings"
	"testing"
)

func TestStableVersionContract(t *testing.T) {
	for _, value := range []string{"0.1.0", "1.2.3", "12.0.99"} {
		if !stableVersion.MatchString(value) {
			t.Fatalf("stable version was rejected: %s", value)
		}
	}
	for _, value := range []string{"v1.2.3", "1.2", "1.2.3-beta.1", "01.2.3"} {
		if stableVersion.MatchString(value) {
			t.Fatalf("non-stable version was accepted: %s", value)
		}
	}
}

func TestAgentInstallContractBootstrapsWindowsWithoutPowerShell(t *testing.T) {
	content, err := os.ReadFile("../../release/agent-install.md.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	document := string(content)
	for _, expected := range []string{
		"do not require PowerShell, npm, or a globally writable package directory",
		"agent-release-manifest.json",
		"verify its SHA-256 against the selected Manifest entry",
		`bootstrap activate --destination "<LocalAppData>\ViceMe\bin\viceme.exe" --agent auto --region <cn-or-global>`,
		"including a POSIX shell such as Git Bash",
		"Do not launch another shell solely for installation",
		"Retain the verified Windows executable for that authorized retry",
	} {
		if !strings.Contains(document, expected) {
			t.Fatalf("agent installation contract is missing %q", expected)
		}
	}
	if strings.Contains(document, "$env:VICEME_REGION='<cn-or-global>'") || strings.Contains(document, "& ./install.ps1") {
		t.Fatal("agent installation contract still routes Windows through PowerShell")
	}
}
