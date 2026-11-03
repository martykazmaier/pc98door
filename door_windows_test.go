//go:build windows

package main

import "testing"

func TestSGRMouseButtons(t *testing.T) {
	var s socketDoor
	s.noteMouse("<0;321;201", 'M')
	s.noteMouse("<32;331;211", 'M')
	s.noteMouse("<0;331;211", 'm')
	got := s.TakeMouse()
	if len(got) != 3 || got[0].x != 320 || got[0].y != 200 || got[0].btn != 1 {
		t.Fatalf("press %+v", got)
	}
	if got[1].x != 330 || got[1].y != 210 || got[1].btn != 1 {
		t.Fatalf("motion %+v", got[1])
	}
	if got[2].btn != 0 {
		t.Fatalf("release %+v", got[2])
	}
}
