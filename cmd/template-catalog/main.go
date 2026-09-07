package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ViceMe-AI/cli/internal/templatecatalog"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("template-catalog", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var source, demoSource, root, output, origin, signingKeyFile, keyID string
	var allowInsecureOrigin bool
	flags.StringVar(&source, "source", "templates/creator-pages/production.json", "source catalog JSON")
	flags.StringVar(&demoSource, "demo-source", "templates/creator-pages/demo.json", "local-only coming-soon catalog JSON")
	flags.StringVar(&root, "root", ".", "repository root for relative template paths")
	flags.StringVar(&output, "output", "", "catalog artifact output directory")
	flags.StringVar(&origin, "origin", "", "public HTTPS catalog origin")
	flags.StringVar(&signingKeyFile, "signing-key-file", "", "base64url PKCS#8 Ed25519 private key file")
	flags.StringVar(&keyID, "key-id", "template-v1", "public signing key identifier")
	flags.BoolVar(&allowInsecureOrigin, "allow-insecure-origin", false, "allow HTTP only for a loopback local demo")
	if err := flags.Parse(args); err != nil || output == "" || origin == "" || signingKeyFile == "" || keyID == "" {
		return errors.New("template-catalog requires --output, --origin, and --signing-key-file")
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	catalog, err := templatecatalog.LoadSourceCatalog(file)
	if err != nil {
		return err
	}
	privateKey, err := loadPrivateKey(signingKeyFile)
	if err != nil {
		return err
	}
	signer := templatecatalog.Signer{KeyID: keyID, PrivateKey: privateKey}
	if allowInsecureOrigin {
		demoFile, err := os.Open(demoSource)
		if err != nil {
			return err
		}
		defer demoFile.Close()
		demos, err := templatecatalog.LoadDemoCatalog(demoFile)
		if err != nil {
			return err
		}
		_, err = templatecatalog.BuildForLocalDemoWithTemplates(root, catalog, demos, output, origin, signer)
		return err
	}
	_, err = templatecatalog.Build(root, catalog, output, origin, signer)
	return err
}

func loadPrivateKey(filename string) (ed25519.PrivateKey, error) {
	encoded, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != strings.TrimSpace(string(encoded)) {
		return nil, errors.New("template-catalog signing key is not canonical base64url")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(raw)
	if err != nil {
		return nil, errors.New("template-catalog signing key is invalid")
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("template-catalog signing key is not Ed25519")
	}
	return privateKey, nil
}
