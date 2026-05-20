// catalog_check verifies that internal/bakery/descriptions.go covers all
// extensions currently published in the live Flatcar Sysext Bakery.
//
// Usage:
//
//	go run ./scripts/catalog_check/           # informational report
//	go run ./scripts/catalog_check/ --strict  # exit 1 if any gaps found
//
// Run before cutting a release to catch new bakery extensions that need
// curated descriptions. Not part of `just ci` (requires network); run
// manually or via `just catalog-check`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/castrojo/knuckle/internal/bakery"
)

func main() {
	strict := flag.Bool("strict", false, "exit 1 if any extensions are missing curated descriptions")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fmt.Println("catalog_check — verifying descriptions.go against live Flatcar Bakery")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Println()

	client := bakery.NewHTTPClient()

	fmt.Print("Fetching live bakery catalog (amd64)... ")
	entries, err := client.FetchCatalogArch(ctx, "amd64")
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nERROR fetching catalog: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("%d extensions found\n\n", len(entries))

	// Sort by name for stable output.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	var missing []bakery.MissingEntry
	var covered int

	for _, e := range entries {
		meta, ok := bakery.Lookup(e.Name)
		if !ok {
			missing = append(missing, bakery.MissingEntry{
				Name:    e.Name,
				Version: e.Version,
				URL:     e.URL,
			})
			fmt.Printf("  MISSING  %-22s  v%s\n", e.Name, e.Version)
		} else {
			covered++
			fmt.Printf("  ok       %-22s  v%-12s  %s · %s\n",
				e.Name, e.Version, meta.SupportTier, meta.Category)
		}
	}

	fmt.Println()
	fmt.Printf("Result: %d covered, %d missing\n", covered, len(missing))

	if len(missing) == 0 {
		fmt.Println()
		fmt.Println("✓ All live bakery extensions have curated descriptions.")
		fmt.Println("  No action needed.")
		return
	}

	fmt.Println()
	fmt.Println("─── Missing entries ────────────────────────────────────────────────────")
	fmt.Printf("Add the following to extensionCatalog in internal/bakery/descriptions.go:\n\n")

	for _, m := range missing {
		fmt.Printf("// %s v%s — source: %s\n", m.Name, m.Version, m.URL)
		fmt.Printf(`"%s": {
	Category:    "TODO", // e.g. "Container Runtime", "Networking", "Orchestration"
	SupportTier: bakery.TierMaintained, // or TierIntegrated, TierExperimental
	Short:       "TODO: one-line description (~80 chars)",
	Long:        "TODO: 3–5 sentence description shown in the detail panel.",
	Caveats:     nil,
},
`, m.Name)
		fmt.Println()
	}

	fmt.Println("─── Checklist ──────────────────────────────────────────────────────────")
	fmt.Println("1. Add the entry/entries above to internal/bakery/descriptions.go")
	fmt.Printf("2. Add %-22q to allKnownExtensions in internal/bakery/descriptions_test.go\n", missing[0].Name)
	fmt.Println("3. Add a row to docs/SYSEXTS.md under the appropriate category")
	fmt.Println("4. Run: just ci")
	fmt.Println()

	if *strict {
		fmt.Fprintf(os.Stderr, "FAIL: %d extension(s) missing curated descriptions (--strict)\n", len(missing))
		os.Exit(1)
	}

	fmt.Printf("⚠ %d extension(s) are missing curated descriptions.\n", len(missing))
	fmt.Println("  Run 'just catalog-check-strict' to enforce as a hard gate.")
}
