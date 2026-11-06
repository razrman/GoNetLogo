package nl

import "testing"

func TestColorDistanceRedMean(t *testing.T) {
	// (r1 + r2) / 2 is 15. r1 + r2/2 is 20. The two formulas disagree.
	r1, g1, b1 := 10, 40, 80
	r2, g2, b2 := 21, 40, 80
	rmean := int64(r1+r2) / 2
	old := int64(r1 + r2/2)
	if rmean == old {
		t.Fatal("channels do not separate (r1+r2)/2 from r1 + r2/2")
	}
	dist := func(mean int64) int64 {
		rd := int64(r1 - r2)
		gd := int64(g1 - g2)
		bd := int64(b1 - b2)
		return (((512 + mean) * rd * rd) >> 8) + 4*gd*gd + (((767 - mean) * bd * bd) >> 8)
	}
	got := colorDistance(r1, g1, b1, r2, g2, b2)
	if got != dist(rmean) {
		t.Fatalf("colorDistance = %d, want %d with rmean=(%d+%d)/2=%d", got, dist(rmean), r1, r2, rmean)
	}
	if got == dist(old) {
		t.Fatal("colorDistance still matches r1 + r2/2")
	}
	if ApproximateRGB(83, 72, 54) != 33 {
		t.Fatalf("approximate-rgb 83 72 54 = %v, want 33", ApproximateRGB(83, 72, 54))
	}
}
