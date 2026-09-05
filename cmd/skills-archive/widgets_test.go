package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
)

func TestSharedWidgetsHaveIdenticalStableAndContentAddressedObjects(t *testing.T) {
	files, manifest, err := buildHostingArchive(cliembed.EmbeddedSkills())
	if err != nil {
		t.Fatal(err)
	}
	if err := addWidgetAssets(files, cliembed.EmbeddedWidgets()); err != nil {
		t.Fatal(err)
	}
	if _, exists := manifest.Skills["_widgets"]; exists {
		t.Fatal("widgets must not become an installable Skill")
	}
	for _, name := range []string{"payment.html", "onboarding.html", "README.md", "qrcodegen.py"} {
		content, err := fs.ReadFile(cliembed.EmbeddedWidgets(), name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		for _, object := range []string{"_widgets/" + name, "_widgets/sha256-" + hex.EncodeToString(digest[:]) + "/" + name} {
			if !bytes.Equal(files[object], content) {
				t.Fatalf("%s does not match embedded source", object)
			}
		}
	}
}
