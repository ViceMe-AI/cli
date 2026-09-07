package main

import (
	"fmt"
	"os"

	"github.com/ViceMe-AI/cli/internal/commerceartifact"
)

func main() {
	keys, err := commerceartifact.ParseTrustRing(os.Getenv("TEMPLATE_CATALOG_TRUST_KEYS"))
	if err != nil || len(keys) == 0 {
		fmt.Fprintln(os.Stderr, "TEMPLATE_CATALOG_TRUST_RING_INVALID")
		os.Exit(1)
	}
}
