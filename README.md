# Сцена

Небольшой сайт на Go: главная со светом, который следует за курсором, регистрация и стена коротких заметок.

## Установка на Windows

Нужны:

- Windows 10 или новее;
- Go 1.22 или новее;
- Git для Windows.

Установите Go и Git, откройте **PowerShell** и выполните:

```powershell
git clone https://github.com/arrieshri/site.git
cd site
go run .
```

Проверить установку Go можно командой:

```powershell
go version
```

Сайт открывается по адресу http://localhost:8080.

Переменные окружения задаются в PowerShell так:

```powershell
$env:PORT = "8080"
$env:SCENE_DB = "data\scene.db"
go run .
```

`PORT` меняет порт (`8080` или `:8080`). `SCENE_DB` задаёт путь к базе. Без неё база создаётся в `data/scene.db`. Каталог `data/` не коммитится в репозиторий.

## Структура проекта

```text
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
└── static/
    ├── boot.js
    ├── motion.wasm
    ├── style.css
    └── wasm_exec.js
```

## Движение

Сценарий движения написан на Go (`motion.go`, `motion_js.go`) и собран в `static/motion.wasm`. После правки этих файлов пересоберите WebAssembly:

```powershell
go generate
```

`static/wasm_exec.js` копируется из установки Go и должен совпадать с компилятором, который собрал wasm.

## Проверка

```powershell
go test ./...
```
