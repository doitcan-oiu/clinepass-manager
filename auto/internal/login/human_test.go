package login

import "testing"

func TestBezierPointsReachTarget(t *testing.T) {
	from := point{X: 10, Y: 10}
	to := point{X: 400, Y: 280}
	pts := bezierPoints(from, to, 16)
	if len(pts) < 8 {
		t.Fatalf("too few points: %d", len(pts))
	}
	last := pts[len(pts)-1]
	if last.X < 390 || last.Y < 270 {
		t.Fatalf("did not reach target: %+v", last)
	}
	if pts[0].X != from.X || pts[0].Y != from.Y {
		t.Fatalf("start %+v", pts[0])
	}
}
