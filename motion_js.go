package main

import (
	"fmt"
	"strconv"
	"strings"
	"syscall/js"
)

// Браузерная оболочка движения. Поведение совпадает с расчётом в motion.go:
// блоки с data-lag догоняют курсор, data-track="pointer" ставит центр блока
// на указатель, #fps показывает кадры в секунду, window.Motion отдаёт
// state, onFrame и rescan.

func main() {
	win := js.Global()
	doc := win.Get("document")

	motionQuery := win.Call("matchMedia", "(prefers-reduced-motion: reduce)")
	reduce := motionQuery.Get("matches").Bool()

	w := win.Get("innerWidth").Float()
	h := win.Get("innerHeight").Float()
	p := pointer{x: w * 0.62, y: h * 0.38, tx: w * 0.62, ty: h * 0.38, dt: 1.0 / 144}

	state := win.Get("Object").New()
	var fps int
	publishPointer(state, p)
	state.Set("fps", fps)

	var (
		items    []domFollower
		frameFns []js.Value
