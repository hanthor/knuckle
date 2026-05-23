package main

import (
	"github.com/projectbluefin/knuckle/internal/bakery"
	"github.com/projectbluefin/knuckle/internal/model"
)

// checkCatalog compares a slice of catalog entries against the curated
// descriptions in internal/bakery/descriptions.go. It returns the number
// of covered entries and a slice of entries that have no curated description.
// The function is pure (no network, no I/O) and designed for unit testing.
func checkCatalog(entries []model.SysextEntry) (covered int, missing []bakery.MissingEntry) {
	for _, e := range entries {
		if _, ok := bakery.Lookup(e.Name); ok {
			covered++
		} else {
			missing = append(missing, bakery.MissingEntry{
				Name:    e.Name,
				Version: e.Version,
				URL:     e.URL,
			})
		}
	}
	return covered, missing
}
