package main

import "testing"

// The commit of a build of the module (go install …@latest): from its
// pseudo-version; none from a tag.
func TestPseudoVersionCommit(t *testing.T) {
	for _, c := range []struct{ v, id, at string }{
		{"v0.0.0-20261101120529-5ff272874732", "5ff272874732", "2026-11-01T12:05:29Z"},
		{"v1.2.4-0.20261101120529-5ff272874732", "5ff272874732", "2026-11-01T12:05:29Z"},
		{"v2.0.1-pre.0.20261101120529-5ff272874732+incompatible", "5ff272874732", "2026-11-01T12:05:29Z"},
		{"v1.2.3", "", ""},
		{"", "", ""},
	} {
		if id, at := pseudoVersionCommit(c.v); id != c.id || at != c.at {
			t.Errorf("%q: %q %q, want %q %q", c.v, id, at, c.id, c.at)
		}
	}
}
