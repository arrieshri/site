package main

import (
	"math"
	"testing"
)

func TestFrameDT(t *testing.T) {
	if got := frameDT(0, 500); math.Abs(got-1.0/144) > 1e-12 {
		t.Fatalf("первый кадр: %v", got)
	}
	if got := frameDT(1000, 1080); math.Abs(got-0.05) > 1e-12 {
		t.Fatalf("ограничение dt: %v", got)
	}
	if got := frameDT(2000, 1000); math.Abs(got-1.0/144) > 1e-12 {
		t.Fatalf("время назад: %v", got)
	}
}

func TestPointerTranslate(t *testing.T) {
	x, y := pointerTranslate(200, 80, 100, 50, 0, 0, 40, 20, "both")
	if math.Abs(x-80) > 1e-9 || math.Abs(y-20) > 1e-9 {
		t.Fatalf("центр: %v %v", x, y)
	}
	x2, y2 := pointerTranslate(200, 80, 180, 70, 80, 20, 40, 20, "both")
	if math.Abs(x2-x) > 1e-9 || math.Abs(y2-y) > 1e-9 {
		t.Fatalf("после сдвига: %v %v", x2, y2)
	}
	x, y = pointerTranslate(200, 80, 100, 50, 0, 0, 40, 20, "x")
	if x != 80 || y != 0 {
		t.Fatalf("ось x: %v %v", x, y)
	}
}
