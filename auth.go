package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "scene_session"
	csrfCookie    = "scene_csrf"
	flashCookie   = "scene_flash"
	sessionTTL    = 14 * 24 * time.Hour
)

var dummyHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("scene-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	dummyHash = h
}

type userContextKey struct{}

type limiter struct {
	mu  sync.Mutex
	hit map[string][]time.Time
}

func newLimiter() *limiter {
	return &limiter{hit: make(map[string][]time.Time)}
}

func (l *limiter) allow(key string, limit int, window time.Duration) bool {
	now := time.Now()
	cut := now.Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hit) > 4096 {
		l.hit = make(map[string][]time.Time)
	}
	prev := l.hit[key]
	kept := prev[:0]
	for _, t := range prev {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.hit[key] = kept
		return false
	}
	l.hit[key] = append(kept, now)
	return true
}

func (s *server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || !validToken(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.userByToken(c.Value)
		if err != nil {
			log.Printf("сессия: %v", err)
			next.ServeHTTP(w, r)
			return
		}
		if u != nil {
			r = r.WithContext(context.WithValue(r.Context(), userContextKey{}, u))
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userContextKey{}).(*User)
	return u
}

func (s *server) view(w http.ResponseWriter, r *http.Request, p page) page {
	p.User = currentUser(r)
	p.CSRF = s.ensureCSRF(w, r)
	return p
}

func (s *server) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && validToken(c.Value) {
		return c.Value
	}
	tok := randHex(32)
	http.SetCookie(w, baseCookie(r, csrfCookie, tok, int(sessionTTL.Seconds())))
	return tok
}

func csrfOK(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || !validToken(c.Value) {
		return false
	}
	sent := r.PostFormValue("csrf")
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(sent)) == 1
}

func (s *server) flash(w http.ResponseWriter, r *http.Request, kind, msg string) {
	http.SetCookie(w, baseCookie(r, flashCookie, url.QueryEscape(kind+":"+msg), 60))
}

func (s *server) takeFlash(w http.ResponseWriter, r *http.Request) (okMsg, errMsg string) {
	c, err := r.Cookie(flashCookie)
	if err != nil || c.Value == "" {
		return "", ""
	}
	http.SetCookie(w, baseCookie(r, flashCookie, "", -1))
	raw, err := url.QueryUnescape(c.Value)
	if err != nil {
		return "", ""
	}
	kind, msg, found := strings.Cut(raw, ":")
	if !found || msg == "" || strings.ContainsAny(msg, "\r\n") {
		return "", ""
	}
	switch kind {
	case "ok":
		return msg, ""
	case "err":
		return "", msg
	default:
		return "", ""
	}
}

func baseCookie(r *http.Request, name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestSecure(r),
	}
}

func requestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func seeOther(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func readForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	return r.ParseForm()
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func validToken(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validEmail(s string) bool {
	if len(s) < 6 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at > 64 || at != strings.LastIndexByte(s, '@') {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if len(domain) < 3 || !strings.Contains(domain, ".") {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") || strings.Contains(local, "..") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '@', r == '.', r == '_', r == '%', r == '+', r == '-':
		default:
			return false
		}
	}
	return true
}

func cleanName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 2 || n > 40 {
		return "", false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '<' || r == '>' {
			return "", false
		}
	}
	return s, true
}

func cleanNote(s string) (string, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	n := utf8.RuneCountInString(s)
	if n < 1 || n > 280 || strings.ContainsRune(s, 0) {
		return "", false
	}
	return s, true
}

func checkPassword(pw string) string {
	if utf8.RuneCountInString(pw) < 8 {
		return "Пароль должен быть не короче 8 символов."
	}
	if len(pw) > 72 {
		return "Пароль слишком длинный."
	}
	return ""
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func passwordMatches(hash, password string) bool {
	if hash == "" {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, err := s.createSession(userID)
	if err != nil {
		return err
	}
	http.SetCookie(w, baseCookie(r, sessionCookie, token, int(sessionTTL.Seconds())))
	return nil
}

func (s *server) registerGet(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		seeOther(w, r, "/account")
		return
	}
	s.render(w, http.StatusOK, "register", s.view(w, r, page{Title: "Регистрация — Сцена"}))
}

func (s *server) registerPost(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		seeOther(w, r, "/account")
		return
	}
	if err := readForm(w, r); err != nil {
		s.renderRegister(w, r, http.StatusBadRequest, "Слишком большой запрос.", "", "")
		return
	}
	nameRaw := r.PostFormValue("name")
	emailRaw := r.PostFormValue("email")
	if !csrfOK(r) {
		s.renderRegister(w, r, http.StatusForbidden, "Форма устарела. Отправьте её ещё раз.", nameRaw, emailRaw)
		return
	}
	if !s.lim.allow("reg:"+clientIP(r), 20, 10*time.Minute) {
		s.renderRegister(w, r, http.StatusTooManyRequests, "Слишком много попыток. Подождите немного.", nameRaw, emailRaw)
		return
	}
	name, nameOK := cleanName(nameRaw)
	email := normalizeEmail(emailRaw)
	password := r.PostFormValue("password")
	if !nameOK {
		s.renderRegister(w, r, http.StatusBadRequest, "Имя: от 2 до 40 символов.", nameRaw, email)
		return
	}
	if !validEmail(email) {
		s.renderRegister(w, r, http.StatusBadRequest, "Почта выглядит неверно.", name, emailRaw)
		return
	}
	if msg := checkPassword(password); msg != "" {
		s.renderRegister(w, r, http.StatusBadRequest, msg, name, email)
		return
	}
	if password != r.PostFormValue("password2") {
		s.renderRegister(w, r, http.StatusBadRequest, "Пароли не совпадают.", name, email)
		return
	}
	existing, _, err := s.userByEmail(email)
	if err != nil {
		log.Printf("пользователь: %v", err)
		s.renderRegister(w, r, http.StatusInternalServerError, "Не удалось сохранить аккаунт.", name, email)
		return
	}
	if existing.ID != 0 {
		s.renderRegister(w, r, http.StatusConflict, "Такая почта уже зарегистрирована.", name, email)
		return
	}
	hash, err := hashPassword(password)
	if err != nil {
		log.Printf("пароль: %v", err)
		s.renderRegister(w, r, http.StatusInternalServerError, "Не удалось сохранить аккаунт.", name, email)
		return
	}
	id, err := s.insertUser(email, name, hash)
	if isUnique(err) {
		s.renderRegister(w, r, http.StatusConflict, "Такая почта уже зарегистрирована.", name, email)
		return
	}
	if err != nil {
		log.Printf("регистрация: %v", err)
		s.renderRegister(w, r, http.StatusInternalServerError, "Не удалось сохранить аккаунт.", name, email)
		return
	}
	if err := s.startSession(w, r, id); err != nil {
		log.Printf("сессия: %v", err)
		s.renderRegister(w, r, http.StatusInternalServerError, "Аккаунт создан, но войти не удалось. Откройте страницу входа.", name, email)
		return
	}
	seeOther(w, r, "/")
}

func (s *server) renderRegister(w http.ResponseWriter, r *http.Request, status int, msg, name, email string) {
	s.render(w, status, "register", s.view(w, r, page{
		Title: "Регистрация — Сцена",
		Error: msg,
		Name:  name,
		Email: email,
	}))
}

func (s *server) loginGet(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		seeOther(w, r, "/")
		return
	}
	s.render(w, http.StatusOK, "login", s.view(w, r, page{Title: "Вход — Сцена"}))
}

func (s *server) loginPost(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		seeOther(w, r, "/")
		return
	}
	if err := readForm(w, r); err != nil {
		s.renderLogin(w, r, http.StatusBadRequest, "Слишком большой запрос.", "")
		return
	}
	email := normalizeEmail(r.PostFormValue("email"))
	if !csrfOK(r) {
		s.renderLogin(w, r, http.StatusForbidden, "Форма устарела. Отправьте её ещё раз.", email)
		return
	}
	if !s.lim.allow("login:"+clientIP(r), 20, 10*time.Minute) {
		s.renderLogin(w, r, http.StatusTooManyRequests, "Слишком много попыток. Подождите немного.", email)
		return
	}
	u, hash, err := s.userByEmail(email)
	if err != nil {
		log.Printf("вход: %v", err)
		s.renderLogin(w, r, http.StatusInternalServerError, "Не удалось войти.", email)
		return
	}
	matches := passwordMatches(hash, r.PostFormValue("password"))
	if u.ID == 0 || !matches {
		s.renderLogin(w, r, http.StatusUnauthorized, "Неверная почта или пароль.", email)
		return
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		log.Printf("сессия: %v", err)
		s.renderLogin(w, r, http.StatusInternalServerError, "Не удалось войти.", email)
		return
	}
	seeOther(w, r, "/")
}

func (s *server) renderLogin(w http.ResponseWriter, r *http.Request, status int, msg, email string) {
	s.render(w, status, "login", s.view(w, r, page{
		Title: "Вход — Сцена",
		Error: msg,
		Email: email,
	}))
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if err := readForm(w, r); err != nil || !csrfOK(r) {
		http.Error(w, "Форма устарела. Обновите страницу.", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.deleteSession(c.Value); err != nil {
			log.Printf("выход: %v", err)
		}
	}
	http.SetCookie(w, baseCookie(r, sessionCookie, "", -1))
	seeOther(w, r, "/")
}

func (s *server) accountGet(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) == nil {
		seeOther(w, r, "/login")
		return
	}
	p := s.view(w, r, page{Title: "Аккаунт — Сцена"})
	p.OK, p.Error = s.takeFlash(w, r)
	s.render(w, http.StatusOK, "account", p)
}

func (s *server) accountName(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		seeOther(w, r, "/login")
		return
	}
	if err := readForm(w, r); err != nil || !csrfOK(r) {
		s.flash(w, r, "err", "Форма устарела. Отправьте её ещё раз.")
		seeOther(w, r, "/account")
		return
	}
	name, ok := cleanName(r.PostFormValue("name"))
	if !ok {
		s.flash(w, r, "err", "Имя: от 2 до 40 символов.")
		seeOther(w, r, "/account")
		return
	}
	if err := s.updateName(u.ID, name); err != nil {
		log.Printf("имя: %v", err)
		s.flash(w, r, "err", "Не удалось сохранить имя.")
		seeOther(w, r, "/account")
		return
	}
	s.flash(w, r, "ok", "Имя сохранено.")
	seeOther(w, r, "/account")
}

func (s *server) accountPassword(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		seeOther(w, r, "/login")
		return
	}
	if err := readForm(w, r); err != nil || !csrfOK(r) {
		s.flash(w, r, "err", "Форма устарела. Отправьте её ещё раз.")
		seeOther(w, r, "/account")
		return
	}
	if !s.lim.allow("pw:"+clientIP(r), 20, 10*time.Minute) {
		s.flash(w, r, "err", "Слишком много попыток. Подождите немного.")
		seeOther(w, r, "/account")
		return
	}
	fresh, hash, err := s.userByEmail(u.Email)
	if err != nil || fresh.ID == 0 {
		log.Printf("пароль: %v", err)
		s.flash(w, r, "err", "Не удалось сменить пароль.")
		seeOther(w, r, "/account")
		return
	}
	if !passwordMatches(hash, r.PostFormValue("current")) {
		s.flash(w, r, "err", "Текущий пароль не подошёл.")
		seeOther(w, r, "/account")
		return
	}
	pw := r.PostFormValue("password")
	if msg := checkPassword(pw); msg != "" {
		s.flash(w, r, "err", msg)
		seeOther(w, r, "/account")
		return
	}
	if pw != r.PostFormValue("password2") {
		s.flash(w, r, "err", "Пароли не совпадают.")
		seeOther(w, r, "/account")
		return
	}
	next, err := hashPassword(pw)
	if err != nil {
		log.Printf("пароль: %v", err)
		s.flash(w, r, "err", "Не удалось сменить пароль.")
		seeOther(w, r, "/account")
		return
	}
	if err := s.updatePassword(fresh.ID, next); err != nil {
		log.Printf("пароль: %v", err)
		s.flash(w, r, "err", "Не удалось сменить пароль.")
		seeOther(w, r, "/account")
		return
	}
	if err := s.deleteUserSessions(fresh.ID); err != nil {
		log.Printf("сессии: %v", err)
		s.flash(w, r, "err", "Пароль изменён, но сессию обновить не удалось. Войдите снова.")
		seeOther(w, r, "/login")
		return
	}
	if err := s.startSession(w, r, fresh.ID); err != nil {
		log.Printf("сессия: %v", err)
		s.flash(w, r, "err", "Пароль изменён. Войдите снова.")
		seeOther(w, r, "/login")
		return
	}
	s.flash(w, r, "ok", "Пароль изменён.")
	seeOther(w, r, "/account")
}

func (s *server) noteCreate(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		seeOther(w, r, "/login")
		return
	}
	if err := readForm(w, r); err != nil {
		s.flash(w, r, "err", "Слишком большой запрос.")
		seeOther(w, r, "/#notes")
		return
	}
	if !csrfOK(r) {
		s.flash(w, r, "err", "Форма устарела. Отправьте её ещё раз.")
		seeOther(w, r, "/#notes")
		return
	}
	if !s.lim.allow("note:"+clientIP(r), 30, 10*time.Minute) {
		s.flash(w, r, "err", "Слишком много заметок за короткое время.")
		seeOther(w, r, "/#notes")
		return
	}
	body, ok := cleanNote(r.PostFormValue("body"))
	if !ok {
		s.flash(w, r, "err", "Заметка должна быть от 1 до 280 символов.")
		seeOther(w, r, "/#notes")
		return
	}
	err := s.addNote(u.ID, body)
	if errors.Is(err, errNoteLimit) {
		s.flash(w, r, "err", "У вас уже 50 заметок. Удалите старую, чтобы добавить новую.")
		seeOther(w, r, "/#notes")
		return
	}
	if err != nil {
		log.Printf("заметка: %v", err)
		s.flash(w, r, "err", "Не удалось сохранить заметку.")
		seeOther(w, r, "/#notes")
		return
	}
	s.flash(w, r, "ok", "Заметка на стене.")
	seeOther(w, r, "/#notes")
}

func (s *server) noteDelete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		seeOther(w, r, "/login")
		return
	}
	if err := readForm(w, r); err != nil || !csrfOK(r) {
		s.flash(w, r, "err", "Форма устарела. Отправьте её ещё раз.")
		seeOther(w, r, "/#notes")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	ok, err := s.deleteNote(u.ID, id)
	if err != nil {
		log.Printf("удаление заметки: %v", err)
		s.flash(w, r, "err", "Не удалось удалить заметку.")
		seeOther(w, r, "/#notes")
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	seeOther(w, r, "/#notes")
}
