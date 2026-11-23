package main

import (
	"database/sql"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed templates static
var content embed.FS

type page struct {
	Title string
	User  *User
	Notes []Note
	CSRF  string
	Error string
	OK    string
	Name  string
	Email string
}

type server struct {
	tmpl *template.Template
	db   *sql.DB
	lim  *limiter
}

func newServer(db *sql.DB) *server {
	return &server{
		tmpl: template.Must(template.ParseFS(content, "templates/*.html")),
		db:   db,
		lim:  newLimiter(),
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})))
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /register", s.registerGet)
	mux.HandleFunc("POST /register", s.registerPost)
	mux.HandleFunc("GET /login", s.loginGet)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /account", s.accountGet)
	mux.HandleFunc("POST /account/name", s.accountName)
	mux.HandleFunc("POST /account/password", s.accountPassword)
	mux.HandleFunc("POST /notes", s.noteCreate)
	mux.HandleFunc("POST /notes/{id}/delete", s.noteDelete)
	return secureHeaders(s.loadUser(mux))
}

func (s *server) home(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	notes, err := s.latestNotes(u)
	if err != nil {
		log.Printf("заметки: %v", err)
		http.Error(w, "Не удалось собрать страницу", http.StatusInternalServerError)
		return
	}
	p := s.view(w, r, page{Title: "Сцена", Notes: notes})
	p.OK, p.Error = s.takeFlash(w, r)
	s.render(w, http.StatusOK, "home", p)
}

func (s *server) render(w http.ResponseWriter, status int, name string, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := s.tmpl.ExecuteTemplate(w, name, p); err != nil {
		log.Printf("шаблон %s: %v", name, err)
	}
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		next.ServeHTTP(w, r)
	})
}

func main() {
	path := dbPath()
	db, err := openDB(path)
	if err != nil {
		log.Fatalf("база: %v", err)
	}
	defer db.Close()

	addr := listenAddr()
	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(newServer(db).routes()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("база sqlite: %s", path)
	log.Printf("сайт открыт: http://localhost%s", addr)
	log.Fatal(srv.ListenAndServe())
}

func listenAddr() string {
	p := os.Getenv("PORT")
	switch {
	case p == "":
		return ":8080"
	case strings.HasPrefix(p, ":"):
		return p
	default:
		return ":" + p
	}
}
