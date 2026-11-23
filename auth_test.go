package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type browser struct {
	h       http.Handler
	cookies map[string]*http.Cookie
}

func newBrowser(h http.Handler) *browser {
	return &browser{h: h, cookies: map[string]*http.Cookie{}}
}

func (b *browser) do(method, target, body string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range b.cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	b.h.ServeHTTP(rr, req)
	for _, c := range rr.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
			continue
		}
		b.cookies[c.Name] = c
	}
	return rr
}

func (b *browser) submit(path string, form url.Values) *httptest.ResponseRecorder {
	c := b.cookies[csrfCookie]
	if c == nil || c.Value == "" {
		panic("нет csrf")
	}
	form.Set("csrf", c.Value)
	return b.do(http.MethodPost, path, form.Encode())
}

func (b *browser) follow(rr *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	if rr.Code != http.StatusSeeOther {
		return rr
	}
	loc, _, _ := strings.Cut(rr.Header().Get("Location"), "#")
	return b.do(http.MethodGet, loc, "")
}

func TestValidation(t *testing.T) {
	if _, ok := cleanName("Анна"); !ok {
		t.Fatal("имя")
	}
	if _, ok := cleanName(" А "); ok {
		t.Fatal("короткое имя")
	}
	if _, ok := cleanName("<b>"); ok {
		t.Fatal("скобки в имени")
	}
	if !validEmail("anna@example.com") || validEmail("not-an-email") || validEmail("a@b") {
		t.Fatal("почта")
	}
	if checkPassword("short") == "" || checkPassword("12345678") != "" {
		t.Fatal("пароль")
	}
	body, ok := cleanNote("  привет <b>  ")
	if !ok || body != "привет <b>" {
		t.Fatalf("заметка %q %v", body, ok)
	}
}

func TestRegisterLoginNote(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	b := newBrowser(h)

	if rr := b.do(http.MethodGet, "/account", ""); rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Fatalf("кабинет без входа: %d %s", rr.Code, rr.Header().Get("Location"))
	}

	page := b.do(http.MethodGet, "/register", "")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Создать аккаунт") {
		t.Fatalf("форма регистрации: %d", page.Code)
	}
	rr := b.submit("/register", url.Values{
		"name":      {"Анна"},
		"email":     {"Anna@Example.com"},
		"password":  {"supersecret"},
		"password2": {"supersecret"},
	})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("регистрация %d: %s", rr.Code, rr.Body.String())
	}
	home := b.follow(rr)
	if !strings.Contains(home.Body.String(), "Добро пожаловать, Анна!") {
		t.Fatalf("приветствие: %s", home.Body.String())
	}

	rr = b.submit("/notes", url.Values{"body": {"привет <b>мир</b>"}})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("заметка %d", rr.Code)
	}
	wall := b.follow(rr).Body.String()
	if !strings.Contains(wall, "привет &lt;b&gt;мир&lt;/b&gt;") || !strings.Contains(wall, "Заметка на стене.") {
		t.Fatalf("стена: %s", wall)
	}
	if strings.Contains(wall, "<b>мир</b>") {
		t.Fatal("html заметки не экранирован")
	}

	id := regexp.MustCompile(`/notes/(\d+)/delete`).FindStringSubmatch(wall)
	if id == nil {
		t.Fatal("нет кнопки удаления")
	}

	other := newBrowser(h)
	other.do(http.MethodGet, "/register", "")
	rr = other.submit("/register", url.Values{
		"name":      {"Борис"},
		"email":     {"boris@example.com"},
		"password":  {"supersecret"},
		"password2": {"supersecret"},
	})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("второй аккаунт %d: %s", rr.Code, rr.Body.String())
	}
	other.follow(rr)
	denied := other.submit("/notes/"+id[1]+"/delete", url.Values{})
	if denied.Code != http.StatusNotFound {
		t.Fatalf("чужое удаление: %d", denied.Code)
	}
	still := b.do(http.MethodGet, "/", "").Body.String()
	if !strings.Contains(still, "привет &lt;b&gt;мир&lt;/b&gt;") {
		t.Fatal("чужая заметка исчезла")
	}

	gone := b.follow(b.submit("/notes/"+id[1]+"/delete", url.Values{}))
	if strings.Contains(gone.Body.String(), "привет &lt;b&gt;") {
		t.Fatal("своя заметка не удалилась")
	}

	out := b.follow(b.submit("/logout", url.Values{}))
	if strings.Contains(out.Body.String(), "Добро пожаловать, Анна!") {
		t.Fatal("после выхода имя осталось")
	}

	b.do(http.MethodGet, "/login", "")
	bad := b.submit("/login", url.Values{
		"email":    {"anna@example.com"},
		"password": {"wrong-password"},
	})
	if bad.Code != http.StatusUnauthorized || !strings.Contains(bad.Body.String(), "Неверная почта или пароль.") {
		t.Fatalf("плохой пароль: %d %s", bad.Code, bad.Body.String())
	}
	ok := b.follow(b.submit("/login", url.Values{
		"email":    {"anna@example.com"},
		"password": {"supersecret"},
	}))
	if !strings.Contains(ok.Body.String(), "Добро пожаловать, Анна!") {
		t.Fatal("повторный вход")
	}

	dup := newBrowser(h)
	dup.do(http.MethodGet, "/register", "")
	again := dup.submit("/register", url.Values{
		"name":      {"Анна"},
		"email":     {"anna@example.com"},
		"password":  {"supersecret"},
		"password2": {"supersecret"},
	})
	if again.Code != http.StatusConflict {
		t.Fatalf("повтор почты: %d", again.Code)
	}

	acc := b.follow(b.do(http.MethodGet, "/account", ""))
	if acc.Code != http.StatusOK || !strings.Contains(acc.Body.String(), "anna@example.com") {
		t.Fatalf("кабинет: %d", acc.Code)
	}
	renamed := b.follow(b.submit("/account/name", url.Values{"name": {"Аня"}}))
	if !strings.Contains(renamed.Body.String(), "Аня") || !strings.Contains(renamed.Body.String(), "Имя сохранено.") {
		t.Fatalf("имя: %s", renamed.Body.String())
	}
	changed := b.follow(b.submit("/account/password", url.Values{
		"current":   {"supersecret"},
		"password":  {"another-secret"},
		"password2": {"another-secret"},
	}))
	if changed.Code != http.StatusOK || !strings.Contains(changed.Body.String(), "Пароль изменён.") {
		t.Fatalf("смена пароля: %d %s", changed.Code, changed.Body.String())
	}
	b.follow(b.submit("/logout", url.Values{}))
	b.do(http.MethodGet, "/login", "")
	back := b.follow(b.submit("/login", url.Values{
		"email":    {"anna@example.com"},
		"password": {"another-secret"},
	}))
	if !strings.Contains(back.Body.String(), "Добро пожаловать, Аня!") {
		t.Fatal("новый пароль не подошёл")
	}
}

func TestCSRFRejected(t *testing.T) {
	s := testServer(t)
	b := newBrowser(s.routes())
	rr := b.do(http.MethodPost, "/register", url.Values{
		"name":      {"Анна"},
		"email":     {"anna@example.com"},
		"password":  {"supersecret"},
		"password2": {"supersecret"},
	}.Encode())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("код %d", rr.Code)
	}
	u, _, err := s.userByEmail("anna@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 0 {
		t.Fatal("пользователь создан без csrf")
	}
}

func TestUserPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scene.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	h := newServer(db).routes()
	b := newBrowser(h)
	b.do(http.MethodGet, "/register", "")
	rr := b.submit("/register", url.Values{
		"name":      {"Нина"},
		"email":     {"nina@example.com"},
		"password":  {"supersecret"},
		"password2": {"supersecret"},
	})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("регистрация %d", rr.Code)
	}
	db.Close()

	db, err = openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h = newServer(db).routes()
	b = newBrowser(h)
	b.do(http.MethodGet, "/login", "")
	home := b.follow(b.submit("/login", url.Values{
		"email":    {"nina@example.com"},
		"password": {"supersecret"},
	}))
	if !strings.Contains(home.Body.String(), "Добро пожаловать, Нина!") {
		t.Fatalf("база не сохранила пользователя: %s", home.Body.String())
	}
}
