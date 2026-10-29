package graph

import "gonetlogo/pkg/nl"

// subCursor is the last frame a subscriber has been handed. Plot samples
// use the same rule as output lines: a replaced unread frame is rebuilt
// from this cursor, so the skipped payload's points are not lost.
//
// It lives outside schema.resolvers.go so `gqlgen generate` does not treat
// it as unknown code and move it to the deletion footer.
type subCursor struct {
	epoch int
	seq   int
	plots map[string]nl.PlotCursor
}
