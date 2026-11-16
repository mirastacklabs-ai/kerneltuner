// Package rulesfs embeds the YAML rule packs.
package rulesfs

import "embed"

// FS holds every rule pack. See docs/DESIGN.md "Rules".
//
//go:embed all:kernel all:ai all:apps all:antipatterns all:ecosystem
var FS embed.FS
