// Package httpapi owns HTTP policy and application workflows. Queries remain
// close to each workflow so transactions and authorization are easy to review.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sweetnet/internal/auth"
	"sweetnet/internal/config"
)

const cookieName = "sweetnet_session"
const SessionLifetime = 30 * 24 * time.Hour

// MaintenanceLock prevents offline file reconciliation while a server runs.
const MaintenanceLock int64 = 83927461

type Server struct {
	DB           *pgxpool.Pool
	Config       config.Config
	limiter      *rateLimiter
	uploads      chan struct{}
	passwordWork chan struct{}
	dummyHash    string
}

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Active      bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

const userColumns = `id::text, username, display_name, role, is_active, created_at`

func scanUser(row pgx.Row) (u User, err error) {
	err = row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Active, &u.CreatedAt)
	return
}

type userKey struct{}

func current(r *http.Request) User { return r.Context().Value(userKey{}).(User) }

func New(db *pgxpool.Pool, cfg config.Config) (*Server, error) {
	if err := os.MkdirAll(cfg.UploadDir, 0700); err != nil {
		return nil, err
	}
	dummy, err := auth.HashPassword("timing-only-not-a-user-password")
	if err != nil {
		return nil, err
	}
	return &Server{DB: db, Config: cfg, limiter: &rateLimiter{entries: map[string]rateEntry{}}, uploads: make(chan struct{}, 2), passwordWork: make(chan struct{}, 2), dummyHash: dummy}, nil
}
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.headers, recoverPanic)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.DB.Ping(ctx) != nil {
			fail(w, 503, "NOT_READY", "Сервис временно недоступен")
			return
		}
		write(w, 200, map[string]string{"status": "ready"})
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.origin)
		r.With(s.boundPasswordWork).Post("/auth/register", s.register)
		r.With(s.boundPasswordWork).Post("/auth/login", s.login)
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Post("/auth/logout", s.logout)
			r.Get("/me", func(w http.ResponseWriter, r *http.Request) { write(w, 200, current(r)) })
			r.Patch("/me", s.profile)
			r.With(s.boundPasswordWork).Post("/me/password", s.password)
			r.Get("/posts", s.listPosts)
			r.Post("/posts", s.createPost)
			r.Get("/posts/{id}", s.getPost)
			r.Patch("/posts/{id}", s.editPost)
			r.Delete("/posts/{id}", s.deletePost)
			r.Get("/media/{id}", s.getMedia)
			r.Route("/admin", func(r chi.Router) {
				r.Use(ownerOnly)
				r.Get("/invites", s.listInvites)
				r.Post("/invites", s.createInvite)
				r.Delete("/invites/{id}", s.revokeInvite)
				r.Get("/users", s.listUsers)
				r.Patch("/users/{id}", s.setActive)
			})
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			fail(w, 404, "NOT_FOUND", "Не найдено")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 405, "METHOD_NOT_ALLOWED", "Метод не поддерживается")
			return
		}
		// Only files from the build directory are served; uploaded files never enter it.
		clean := filepath.Clean("/" + r.URL.Path)
		filename := filepath.Join(s.Config.WebDir, clean)
		info, err := os.Stat(filename)
		if err == nil && !info.IsDir() {
			http.ServeFile(w, r, filename)
			return
		}
		if strings.HasPrefix(clean, "/assets/") || filepath.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.Config.WebDir, "index.html"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		fail(w, 405, "METHOD_NOT_ALLOWED", "Метод не поддерживается")
	})
	return r
}
func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if s.Config.Production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				slog.Error("request panic")
				fail(w, 500, "INTERNAL_ERROR", "Внутренняя ошибка")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) origin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && r.Header.Get("Origin") != s.Config.Origin {
			fail(w, 403, "ORIGIN_REJECTED", "Запрос с этого адреса запрещён")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || len(c.Value) > 128 {
			fail(w, 401, "UNAUTHENTICATED", "Войдите в аккаунт")
			return
		}
		u, err := scanUser(s.DB.QueryRow(r.Context(), `SELECT u.id::text,u.username,u.display_name,u.role,u.is_active,u.created_at FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.is_active`, auth.HashSecret(c.Value)))
		if errors.Is(err, pgx.ErrNoRows) {
			s.clearCookie(w)
			fail(w, 401, "UNAUTHENTICATED", "Сессия завершена. Войдите снова")
			return
		}
		if err != nil {
			dbError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}
func ownerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if current(r).Role != "owner" {
			fail(w, 403, "FORBIDDEN", "Недостаточно прав")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func dbError(w http.ResponseWriter, err error) {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		fail(w, 409, "CONFLICT", "Такая запись уже существует")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "NOT_FOUND", "Не найдено")
		return
	}
	slog.Error("database operation failed")
	fail(w, 500, "INTERNAL_ERROR", "Не удалось выполнить запрос")
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(w, 415, "UNSUPPORTED_TYPE", "Ожидается JSON")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			fail(w, 413, "TOO_LARGE", "Запрос слишком большой")
		} else {
			fail(w, 400, "VALIDATION_ERROR", "Проверьте поля запроса")
		}
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "VALIDATION_ERROR", "Ожидается один JSON-объект")
		return false
	}
	return true
}
func resourceID(w http.ResponseWriter, r *http.Request) (string, bool) {
	v := chi.URLParam(r, "id")
	if _, err := uuid.Parse(v); err != nil {
		fail(w, 404, "NOT_FOUND", "Не найдено")
		return "", false
	}
	return v, true
}
func (s *Server) setCookie(w http.ResponseWriter, secret string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: secret, Path: "/", HttpOnly: true, Secure: s.Config.Production || strings.HasPrefix(s.Config.Origin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: int(SessionLifetime.Seconds()), Expires: time.Now().Add(SessionLifetime)})
}
func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.Config.Production || strings.HasPrefix(s.Config.Origin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

// Bounded fixed-window limiter. Never trusts forwarded addresses; a same-host
// reverse proxy shares the IP bucket, while username/user buckets remain distinct.
type rateEntry struct {
	count int
	until time.Time
}
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
}

func (l *rateLimiter) allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) >= 10000 {
		for k, v := range l.entries {
			if now.After(v.until) {
				delete(l.entries, k)
			}
		}
		if _, exists := l.entries[key]; !exists && len(l.entries) >= 10000 {
			return false
		}
	}
	v := l.entries[key]
	if now.After(v.until) {
		v = rateEntry{until: now.Add(window)}
	}
	v.count++
	l.entries[key] = v
	return v.count <= max
}
func (s *Server) limit(w http.ResponseWriter, key string, n int, d time.Duration) bool {
	if s.limiter.allow(auth.HashSecret(key), n, d) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	fail(w, 429, "RATE_LIMITED", "Слишком много запросов. Попробуйте позже")
	return false
}
func ip(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Argon2 uses memory intentionally. Limit concurrent password work globally,
// in addition to time-window limits, to keep unauthenticated traffic bounded.
func (s *Server) boundPasswordWork(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.passwordWork <- struct{}{}:
			defer func() { <-s.passwordWork }()
			next.ServeHTTP(w, r)
		default:
			fail(w, 429, "BUSY", "Сервер занят. Попробуйте позже")
		}
	})
}
