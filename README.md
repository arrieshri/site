# Сцена

Небольшой сайт на Go: главная со светом, который следует за курсором, регистрация и стена коротких заметок.

## Запуск

Нужен Go 1.22+.

```bash
git clone https://github.com/arrieshri/site.git
cd site
go run .

## Запуск

site/
├── main.go
├── auth.go
├── db.go
├── motion.go
├── motion_js.go
├── ... (остальные .go файлы)
├── go.mod
├── go.sum
├── templates/
│   ├── auth.html
│   ├── home.html
│   └── layout.html
├── static/
│   ├── boot.js
│   ├── motion.wasm
│   ├── style.css
│   └── wasm_exec.js
└── data/          ← эта папка должна быть в .gitignore

```
go run .
```

Сайт открывается на http://localhost:8080

`PORT` меняет порт (`8080` или `:8080`). `SCENE_DB` задаёт путь к базе. Без неё база создаётся в `data/scene.db`. Каталог `data/` не коммитьте: в нём хеши паролей и текст заметок.

## Движение

Сценарий движения написан на Go (`motion.go`, `motion_js.go`) и собран в `static/motion.wasm`. После правки этих файлов пересоберите его той же версией Go, которой запускаете сайт:

```
go generate
```

`static/wasm_exec.js` копируется из установки Go и должен совпадать с компилятором, который собрал wasm.

## Проверка

```
go test ./...
```
