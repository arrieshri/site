package main

import (
	"math"
	"strconv"
	"strings"
)

// Движение на частоте экрана: шаг считается через dt, поэтому скорость
// не зависит от герцовки монитора.

const pointerRate = 32

type pointer struct {
	x, y   float64
	tx, ty float64
	nx, ny float64
	dt     float64
}

func (p *pointer) advance(reduce bool, dt, innerW, innerH float64) {
	p.dt = dt
	blend := expBlend(pointerRate, dt, reduce)
	p.x += (p.tx - p.x) * blend
	p.y += (p.ty - p.y) * blend
	if innerW > 0 {
		p.nx = p.x/innerW - 0.5
	}
	if innerH > 0 {
		p.ny = p.y/innerH - 0.5
	}
}

type follower struct {
	lag   float64
	rang  float64
	track bool
	axis  string
	x, y  float64
}

func newFollower(lag, rang float64, track bool, axis string, px, py float64) follower {
	f := follower{lag: lag, rang: rang, track: track, axis: axis}
	if track && axis != "y" {
		f.x = px
	}
	if track && axis != "x" {
		f.y = py
	}
	return f
}

func (f *follower) advance(reduce bool, dt, px, py, nx, ny, innerW, innerH float64) {
	blend := expBlend(f.lag, dt, reduce)
	var tx, ty float64
	if f.track {
		if f.axis != "y" {
			tx = px
		}
		if f.axis != "x" {
			ty = py
		}
	} else {
		if f.axis != "y" {
			tx = nx * f.rang * innerW
		}
		if f.axis != "x" {
			ty = ny * f.rang * innerH
		}
	}
	f.x += (tx - f.x) * blend
	f.y += (ty - f.y) * blend
}

func (f follower) transform() string {
	switch f.axis {
	case "x":
		return "translate3d(" + px2(f.x) + "px,0,0)"
	case "y":
		return "translate3d(0," + px2(f.y) + "px,0)"
	default:
		return "translate3d(" + px2(f.x) + "px," + px2(f.y) + "px,0)"
	}
}

func frameDT(last, now float64) float64 {
	if last <= 0 || now <= last {
		return 1.0 / 144
	}
	return math.Min(0.05, (now-last)/1000)
}

// pointerTranslate — сдвиг, который ставит центр элемента на курсор.
// rectLeft и rectTop уже включают текущий translate curX, curY.
func pointerTranslate(px, py, rectLeft, rectTop, curX, curY, width, height float64, axis string) (float64, float64) {
	var x, y float64
	if axis != "y" {
		x = px - (rectLeft - curX) - width/2
	}
	if axis != "x" {
		y = py - (rectTop - curY) - height/2
	}
	return x, y
}

func expBlend(rate, dt float64, reduce bool) float64 {
	if reduce {
		return 1
	}
	return 1 - math.Exp(-rate*dt)
}

func roundFPS(frames int, elapsed float64) int {
	return int(math.Round(float64(frames) * 1000 / elapsed))
}

func lagFrom(raw string, ok bool) float64 {
	f, good := parseDataFloat(raw, ok)
	if !good || f <= 0 || math.IsInf(f, 0) {
		return 12
	}
	return f
}

func rangeFrom(raw string, ok bool) float64 {
	f, good := parseDataFloat(raw, ok)
	if !good || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func axisFrom(raw string, ok bool) string {
	if !ok {
		return "both"
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "x":
		return "x"
	case "y":
		return "y"
	default:
		return "both"
	}
}

func parseDataFloat(raw string, ok bool) (float64, bool) {
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(f) {
		return 0, false
	}
	return f, true
}

func px2(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
