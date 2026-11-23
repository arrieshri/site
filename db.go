package main

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// База — один файл SQLite рядом с сайтом. Облако и ключ API не нужны.
// Путь меняется переменной SCENE_DB.

const timeLayout = "2006-01-02T15:04:05Z"

const maxNotesPerUser = 50

var errNoteLimit = errors.New("слишком много заметок")

type User struct {
	ID        int64
	Email     string
	Name      string
	CreatedAt time.Time
}

func (u User) Since() string {
	return u.CreatedAt.In(time.Local).Format("02.01.2006")
}
