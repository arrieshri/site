package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) *server {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "scene.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return newServer(db)
}

func TestHome(t *testing.T) {
	h := testServer(t).routes()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("код %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Добро пожаловать!", "Листай вниз", `href="#next"`,
		"/static/wasm_exec.js", "/static/boot.js", "/static/motion.wasm", "/static/style.css",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("на странице нет %q", want)
		}
	}
	if strings.Contains(body, "app.js") {
		t.Fatal("страница всё ещё ссылается на app.js")
	}
	if strings.Contains(body, "hair") {
		t.Fatal("на странице осталось перекрестие курсора")
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type %s", ct)
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("frame %s", rr.Header().Get("X-Frame-Options"))
	}
	if !strings.Contains(body, "Регистрация") || !strings.Contains(body, "Пока на стене пусто") {
		t.Fatal("на странице нет входа или стены")
	}
}

func TestMotionAssets(t *testing.T) {
	h := testServer(t).routes()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/motion.wasm", nil))
	wasm := rr.Body.Bytes()
	if rr.Code != http.StatusOK || len(wasm) < 4 || string(wasm[:4]) != "\x00asm" {
		t.Fatalf("motion.wasm: код %d, размер %d", rr.Code, len(wasm))
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/wasm") {
		t.Fatalf("motion.wasm content-type %s", ct)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/wasm_exec.js", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "globalThis.Go") {
		t.Fatalf("wasm_exec.js: код %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/boot.js", nil))
	boot := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(boot, "motion.wasm") || !strings.Contains(boot, "new Go") {
		t.Fatalf("boot.js: код %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/static/style.css", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), ".stage") {
		t.Fatalf("style.css: код %d", rr.Code)
	}
}

func TestMissing(t *testing.T) {
	h := testServer(t).routes()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/netu", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("код %d", rr.Code)
	}
}
