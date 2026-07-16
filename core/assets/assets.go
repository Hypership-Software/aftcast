// Package assets embeds files shipped inside the aftcast binary. Currently the
// starter Cedar policy pack that `aftcast init` writes into a fresh install and
// the daemon loads as the default policy. Kept in its own package because
// go:embed can only reach files at or below the embedding source file.
package assets

import "embed"

//go:embed starter-pack/*.cedar
var StarterPack embed.FS

// StarterPackDir is the path within StarterPack that holds the *.cedar files.
const StarterPackDir = "starter-pack"
