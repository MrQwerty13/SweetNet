// admin is a local-only tool. Credentials are read from a hidden terminal or stdin.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
	"sweetnet/internal/auth"
	"sweetnet/internal/httpapi"
	"sweetnet/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: admin migrate | bootstrap | reset-password | reconcile-media")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if os.Args[1] == "migrate" {
		if err := migrations.Up(ctx, url); err != nil {
			return errors.New("migration failed; verify database availability and schema")
		}
		fmt.Println("Миграции применены")
		return nil
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()
	if db.Ping(ctx) != nil {
		return errors.New("database unavailable")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	username := fs.String("username", "", "username")
	name := fs.String("display-name", "", "display name")
	apply := fs.Bool("apply", false, "remove orphan images (server must be stopped)")
	if err = fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch os.Args[1] {
	case "bootstrap", "reset-password":
		user, ok := auth.Username(*username)
		if !ok {
			return errors.New("username: 3–32 letters a-z, digits or underscore")
		}
		if os.Args[1] == "bootstrap" && !auth.ValidName(*name) {
			return errors.New("display-name: 1–80 characters")
		}
		password, err := readPassword()
		if err != nil {
			return err
		}
		if !auth.ValidPassword(password) {
			return errors.New("password must contain 12–128 characters")
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return errors.New("password hashing failed")
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			return errors.New("cannot start transaction")
		}
		defer tx.Rollback(ctx)
		if os.Args[1] == "bootstrap" {
			// Unique partial index also protects concurrent bootstrap processes.
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE role='owner')`).Scan(&exists); err != nil {
				return errors.New("cannot read users; apply migrations first")
			}
			if exists {
				return errors.New("owner already exists")
			}
			_, err = tx.Exec(ctx, `INSERT INTO users(username,display_name,password_hash,role) VALUES($1,$2,$3,'owner')`, user, *name, hash)
			if err != nil {
				return errors.New("owner creation conflict or database error")
			}
		} else {
			var id string
			err = tx.QueryRow(ctx, `UPDATE users SET password_hash=$1,updated_at=now() WHERE username=$2 RETURNING id::text`, hash, user).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return errors.New("user not found")
			}
			if err != nil {
				return errors.New("password reset failed")
			}
			if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
				return errors.New("session revocation failed")
			}
		}
		if tx.Commit(ctx) != nil {
			return errors.New("transaction commit failed")
		}
		fmt.Println("Готово")
		return nil
	case "reconcile-media":
		return reconcile(ctx, db, *apply)
	default:
		return errors.New("unknown command")
	}
}
func readPassword() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Пароль: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	// A single bounded line, avoiding unbounded input and argv/history exposure.
	r := bufio.NewReaderSize(os.Stdin, 1024)
	lineBytes, err := r.ReadSlice('\n')
	line := string(lineBytes)
	if errors.Is(err, bufio.ErrBufferFull) {
		return "", errors.New("password input too long")
	}
	if err != nil && len(line) == 0 {
		return "", errors.New("cannot read password from stdin")
	}
	if len(line) > 1024 {
		return "", errors.New("password input too long")
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}
func reconcile(ctx context.Context, db *pgxpool.Pool, apply bool) error {
	dir := os.Getenv("UPLOAD_DIR")
	if dir == "" {
		return errors.New("UPLOAD_DIR is required")
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, httpapi.MaintenanceLock).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return errors.New("stop the application before reconciling media")
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, httpapi.MaintenanceLock)
	rows, err := conn.Query(ctx, `SELECT storage_key FROM post_images`)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		known[key] = true
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	orphans, missing := 0, 0
	for _, f := range files {
		if known[f.Name()] {
			delete(known, f.Name())
			continue
		}
		if !f.Type().IsRegular() {
			continue
		}
		ext := filepath.Ext(f.Name())
		if ext != ".png" && ext != ".jpeg" {
			continue
		}
		if _, err = uuid.Parse(strings.TrimSuffix(f.Name(), ext)); err != nil {
			continue
		}
		orphans++
		if apply {
			if err = os.Remove(filepath.Join(dir, f.Name())); err != nil {
				return errors.New("cannot remove orphan image")
			}
		}
	}
	missing = len(known)
	fmt.Printf("Сироты: %d; отсутствующие файлы: %d; применено: %t\n", orphans, missing, apply)
	if missing > 0 {
		return errors.New("database references missing images; restore from backup")
	}
	return nil
}
