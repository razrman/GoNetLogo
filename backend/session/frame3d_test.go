package session

import (
	"encoding/base64"
	"encoding/binary"
	"image/color"
	"testing"

	"gonetlogo/pkg/nl"
)

// TestVoxelsPacking checks the 3D voxel buffer byte layout directly,
// including negative coordinates, and that a 2D world sends none.
func TestVoxelsPacking(t *testing.T) {
	w := nl.NewWorld3D(-3, 3, -3, 3, -2, 4)
	w.GetPatch3(-3, 2, -2).PColor = 15
	w.GetPatch3(3, -3, 4).PColor = 55
	raw, err := base64.StdEncoding.DecodeString(voxels(w))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2*voxelBytes {
		t.Fatalf("%d bytes, want %d", len(raw), 2*voxelBytes)
	}
	at := func(i, o int) int { return int(int16(binary.LittleEndian.Uint16(raw[i*voxelBytes+o:]))) }
	// Patch order lists the top layer (pzcor 4) first.
	if at(0, 0) != 3 || at(0, 2) != -3 || at(0, 4) != 4 {
		t.Fatalf("voxel 0 at %d %d %d", at(0, 0), at(0, 2), at(0, 4))
	}
	if at(1, 0) != -3 || at(1, 2) != 2 || at(1, 4) != -2 {
		t.Fatalf("voxel 1 at %d %d %d", at(1, 0), at(1, 2), at(1, 4))
	}
	green := nl.ColorRGBA(55)
	if raw[6] != green.R || raw[7] != green.G || raw[8] != green.B || raw[9] != 255 {
		t.Fatalf("voxel 0 color %v, want %v alpha 255", raw[6:10], green)
	}
	if got := voxels(nl.NewWorld(-3, 3, -3, 3, true, true)); got != "" {
		t.Fatalf("2D world voxels %q", got)
	}
}

// TestVoxelAlpha checks that an RGBA patch keeps its alpha and that a
// fully transparent patch is left out of the 3D buffer.
func TestVoxelAlpha(t *testing.T) {
	w := nl.NewWorld3D(-1, 1, -1, 1, -1, 1)
	glass := w.GetPatch3(1, 0, 1)
	glass.HasRGB = true
	glass.RGB = color.RGBA{R: 10, G: 20, B: 30, A: 128}
	clear := w.GetPatch3(-1, 0, 0)
	clear.HasRGB = true
	clear.RGB = color.RGBA{R: 9, G: 9, B: 9, A: 0}
	raw, err := base64.StdEncoding.DecodeString(voxels(w))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != voxelBytes {
		t.Fatalf("%d bytes, want one voxel (%d)", len(raw), voxelBytes)
	}
	if int(int16(binary.LittleEndian.Uint16(raw[0:]))) != 1 || raw[9] != 128 {
		t.Fatalf("voxel %v", raw)
	}
	if raw[6] != 10 || raw[7] != 20 || raw[8] != 30 {
		t.Fatalf("rgb %v", raw[6:9])
	}
}

// TestWorldFrame2DUnchanged pins the 3D fields of a 2D frame to their
// empty values, so the 2D canvas sees the same data as before.
func TestWorldFramePerspective(t *testing.T) {
	w := nl.NewWorld3D(-5, 5, -5, 5, -5, 5)
	t0 := w.CreateTurtle("", 1, 2, 3, 90)
	w.SetPerspective("follow", t0)
	f := worldLocked(w)
	if f.Perspective != "follow" || f.Subject == nil || *f.Subject != t0.Who {
		t.Fatalf("follow perspective %q subject %v", f.Perspective, f.Subject)
	}
	w.SetPerspective("ride", t0)
	f = worldLocked(w)
	if f.Perspective != "ride" || f.Subject == nil || *f.Subject != t0.Who {
		t.Fatalf("ride perspective %q subject %v", f.Perspective, f.Subject)
	}
	w.SetPerspective("watch", t0)
	f = worldLocked(w)
	if f.Perspective != "watch" || f.Subject == nil || *f.Subject != t0.Who {
		t.Fatalf("watch perspective %q subject %v", f.Perspective, f.Subject)
	}
	w.ResetPerspective()
	f = worldLocked(w)
	if f.Perspective != "" || f.Subject != nil {
		t.Fatalf("reset perspective %q subject %v", f.Perspective, f.Subject)
	}
}

func TestWorldFrame2DUnchanged(t *testing.T) {
	w := nl.NewWorld(-2, 2, -2, 2, true, false)
	tt := w.CreateTurtle("", 1, 1, 0, 45)
	other := w.CreateTurtle("", 0, 0, 0, 0)
	if _, _, err := w.CreateLink(tt, other, false, ""); err != nil {
		t.Fatal(err)
	}
	f := worldLocked(w)
	if f.Depth != 1 || f.MinPzcor != 0 || f.MaxPzcor != 0 || f.WrapZ || f.Voxels != "" || f.Observer != nil {
		t.Fatalf("2D frame 3D fields %+v", f)
	}
	if f.Perspective != "" || f.Subject != nil {
		t.Fatalf("2D perspective %q subject %v", f.Perspective, f.Subject)
	}
	if f.PatchColors == "" || len(f.Turtles) != 2 || f.Turtles[0].Z != 0 || f.Turtles[0].Pitch != 0 || f.Links[0].Z1 != 0 {
		t.Fatalf("2D frame %+v", f)
	}
	if f.Turtles[0].Heading != 45 || f.Turtles[0].X != 1 {
		t.Fatalf("2D turtle %+v", f.Turtles[0])
	}
}

// BenchmarkWorldFrame3DDense is the frame cost of the largest bundled 3D
// world (Percolation 3D, 43x43x103) with every patch colored.
func BenchmarkWorldFrame3DDense(b *testing.B) {
	w := nl.NewWorld3D(-21, 21, -21, 21, -51, 51)
	for _, a := range w.Patches().Agents {
		a.(*nl.Patch).PColor = 45
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = worldLocked(w)
	}
}
