package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"sweetnet/internal/auth"
	"sweetnet/internal/config"
	"sweetnet/migrations"
)

type fixture struct {
	t        *testing.T
	db       *pgxpool.Pool
	s        *Server
	h        http.Handler
	owner    *http.Cookie
	member   *http.Cookie
	ownerID  string
	memberID string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration tests not run")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	var dbName string
	if err = base.QueryRow(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatal("integration tests require database name ending _test")
	}
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	pc, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	pc.ConnConfig.RuntimeParams["search_path"] = schema
	// Migrate with the same isolated search_path, never truncate shared tables.
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	isolatedURL := url + sep + "search_path=" + schema
	if err = migrations.Up(ctx, isolatedURL); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Up(ctx, isolatedURL); err != nil {
		t.Fatal("repeat migration", err)
	}
	db, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		cleanup, _ := pgxpool.New(ctx, url)
		if cleanup != nil {
			defer cleanup.Close()
			cleanup.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		}
	})
	cfg := config.Config{Origin: "http://localhost:8080", UploadDir: t.TempDir(), WebDir: t.TempDir()}
	s, err := New(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("correct-password-123")
	var id string
	if err = db.QueryRow(ctx, `INSERT INTO users(username,display_name,password_hash,role) VALUES('owner','Владелец',$1,'owner') RETURNING id::text`, hash).Scan(&id); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, db: db, s: s, h: s.Handler(), ownerID: id}
	resp := f.json("POST", "/api/v1/auth/login", nil, `{"username":"owner","password":"correct-password-123"}`, 200)
	f.owner = resp.Result().Cookies()[0]
	token := f.invite()
	resp = f.json("POST", "/api/v1/auth/register", nil, fmt.Sprintf(`{"token":%q,"username":"member","display_name":"Друг","password":"correct-password-123"}`, token), 201)
	f.member = resp.Result().Cookies()[0]
	var member User
	json.Unmarshal(resp.Body.Bytes(), &member)
	f.memberID = member.ID
	return f
}
func (f *fixture) req(method, path string, cookie *http.Cookie, body io.Reader, ct, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	r.RemoteAddr = "127.0.0.1:12345"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if ct != "" {
		r.Header.Set("Content-Type", ct)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}
func (f *fixture) json(method, path string, cookie *http.Cookie, body string, status int) *httptest.ResponseRecorder {
	f.t.Helper()
	w := f.req(method, path, cookie, strings.NewReader(body), "application/json", f.s.Config.Origin)
	if w.Code != status {
		f.t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w
}
func (f *fixture) invite() string {
	f.t.Helper()
	w := f.json("POST", "/api/v1/admin/invites", f.owner, "", 201)
	var i Invite
	json.Unmarshal(w.Body.Bytes(), &i)
	return strings.Split(i.URL, "token=")[1]
}
func (f *fixture) upload(cookie *http.Cookie, text string, images int, ct string) *httptest.ResponseRecorder {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	mw.WriteField("body", text)
	var im bytes.Buffer
	png.Encode(&im, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	for i := 0; i < images; i++ {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="images"; filename="%d.png"`, i))
		header.Set("Content-Type", ct)
		part, _ := mw.CreatePart(header)
		part.Write(im.Bytes())
	}
	mw.Close()
	return f.req("POST", "/api/v1/posts", cookie, &b, mw.FormDataContentType(), f.s.Config.Origin)
}
func (f *fixture) post(cookie *http.Cookie, body string, photos int) Post {
	f.t.Helper()
	w := f.upload(cookie, body, photos, "image/png")
	if w.Code != 201 {
		f.t.Fatalf("create post: %d %s", w.Code, w.Body.String())
	}
	var p Post
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func TestClosedCircle(t *testing.T) {
	f := setup(t)
	t.Run("membership and secure cookie", func(t *testing.T) {
		if !f.owner.HttpOnly || f.owner.SameSite != http.SameSiteLaxMode || f.owner.Path != "/" {
			t.Fatal("cookie policy")
		}
		f.json("GET", "/api/v1/me", f.owner, "", 200)
		f.json("GET", "/api/v1/me", nil, "", 401)
		f.json("GET", "/api/v1/admin/users", f.member, "", 403)
		f.json("POST", "/api/v1/admin/invites", f.member, "", 403)
		f.json("PATCH", "/api/v1/me", f.member, `{"display_name":"x","role":"owner"}`, 400)
		f.json("PATCH", "/api/v1/admin/users/"+f.ownerID, f.owner, `{"is_active":false}`, 403)
	})
	t.Run("csrf and public route isolation", func(t *testing.T) {
		for _, origin := range []string{"", "https://evil.example"} {
			if w := f.req("POST", "/api/v1/auth/login", nil, strings.NewReader(`{}`), "application/json", origin); w.Code != 403 {
				t.Fatal("origin accepted")
			}
		}
		f.json("GET", "/api/missing", nil, "", 404)
		f.json("GET", "/readyz", nil, "", 200)
	})
	t.Run("invite cannot be reused", func(t *testing.T) {
		token := f.invite()
		body := fmt.Sprintf(`{"token":%q,"username":"once","display_name":"Один","password":"correct-password-123"}`, token)
		f.json("POST", "/api/v1/auth/register", nil, body, 201)
		f.json("POST", "/api/v1/auth/register", nil, strings.Replace(body, "once", "twice", 1), 400)
	})
	t.Run("concurrent invitation acceptance", func(t *testing.T) {
		token := f.invite()
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				w := f.req("POST", "/api/v1/auth/register", nil, strings.NewReader(fmt.Sprintf(`{"token":%q,"username":"race%d","display_name":"Race","password":"correct-password-123"}`, token, i)), "application/json", f.s.Config.Origin)
				codes <- w.Code
			}(i)
		}
		wg.Wait()
		close(codes)
		success, reject := 0, 0
		for c := range codes {
			if c == 201 {
				success++
			} else if c == 400 {
				reject++
			}
		}
		if success != 1 || reject != 1 {
			t.Fatalf("success=%d reject=%d", success, reject)
		}
	})
	t.Run("expired and revoked invitations", func(t *testing.T) {
		for _, clause := range []string{"expires_at=now()-interval '1 day'", "revoked_at=now()"} {
			token := f.invite()
			f.db.Exec(context.Background(), "UPDATE invites SET "+clause+" WHERE token_hash=$1", auth.HashSecret(token))
			f.json("POST", "/api/v1/auth/register", nil, fmt.Sprintf(`{"token":%q,"username":"expired","display_name":"X","password":"correct-password-123"}`, token), 400)
		}
	})
	p := f.post(f.owner, "Привет, друзья!", 1)
	t.Run("shared feed and private media", func(t *testing.T) {
		if len(p.Images) != 1 {
			t.Fatal("missing image")
		}
		w := f.json("GET", "/api/v1/posts", f.member, "", 200)
		if !strings.Contains(w.Body.String(), p.ID) {
			t.Fatal("friend cannot see post")
		}
		for _, path := range []string{"/api/v1/posts", "/api/v1/posts/" + p.ID, p.Images[0].URL} {
			f.json("GET", path, nil, "", 401)
			w = f.json("GET", path, f.member, "", 200)
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache policy")
			}
		}
		f.json("PATCH", "/api/v1/posts/"+p.ID, f.member, `{"body":"hacked"}`, 403)
		f.json("DELETE", "/api/v1/posts/"+p.ID, f.member, "", 403)
		f.json("PATCH", "/api/v1/posts/"+p.ID, f.owner, `{"body":"edited"}`, 200)
	})
	t.Run("empty and invalid uploads", func(t *testing.T) {
		for _, test := range []struct {
			text string
			n    int
			mime string
			want int
		}{{"", 0, "image/png", 400}, {"ok", 5, "image/png", 400}, {"ok", 1, "image/jpeg", 415}, {strings.Repeat("я", 5001), 0, "image/png", 400}} {
			w := f.upload(f.member, test.text, test.n, test.mime)
			if w.Code != test.want {
				t.Fatalf("got %d want %d", w.Code, test.want)
			}
		}
		empty := f.post(f.member, "", 1)
		f.json("GET", "/api/v1/posts/"+empty.ID, f.owner, "", 200)
		text := f.post(f.member, "text", 0)
		f.json("PATCH", "/api/v1/posts/"+text.ID, f.member, `{"body":" "}`, 400)
		f.json("PATCH", "/api/v1/posts/"+text.ID, f.owner, `{"body":"owner edit"}`, 403)
		f.json("DELETE", "/api/v1/posts/"+empty.ID, f.owner, "", 204)
		f.json("GET", empty.Images[0].URL, f.member, "", 404)
	})
	t.Run("pagination ties", func(t *testing.T) {
		for range 4 {
			f.post(f.owner, "same date", 0)
		}
		f.db.Exec(context.Background(), `UPDATE posts SET created_at='2026-01-01T00:00:00Z'`)
		seen := map[string]bool{}
		path := "/api/v1/posts?limit=2"
		for page := 0; page < 10; page++ {
			w := f.json("GET", path, f.member, "", 200)
			var feed struct {
				Items []Post  `json:"items"`
				Next  *string `json:"next_cursor"`
			}
			json.Unmarshal(w.Body.Bytes(), &feed)
			for _, p := range feed.Items {
				if seen[p.ID] {
					t.Fatal("duplicate page item")
				}
				seen[p.ID] = true
			}
			if feed.Next == nil {
				break
			}
			path = "/api/v1/posts?limit=2&cursor=" + *feed.Next
		}
		var count int
		f.db.QueryRow(context.Background(), `SELECT count(*) FROM posts`).Scan(&count)
		if len(seen) != count {
			t.Fatalf("seen %d of %d", len(seen), count)
		}
		f.json("GET", "/api/v1/posts?cursor=invalid", f.member, "", 400)
	})
	t.Run("disable invalidates and reenable does not revive", func(t *testing.T) {
		f.json("PATCH", "/api/v1/admin/users/"+f.memberID, f.owner, `{"is_active":false}`, 200)
		f.json("GET", p.Images[0].URL, f.member, "", 401)
		f.json("POST", "/api/v1/auth/login", nil, `{"username":"member","password":"correct-password-123"}`, 401)
		f.json("PATCH", "/api/v1/admin/users/"+f.memberID, f.owner, `{"is_active":true}`, 200)
		f.json("GET", "/api/v1/me", f.member, "", 401)
		w := f.json("POST", "/api/v1/auth/login", nil, `{"username":"member","password":"correct-password-123"}`, 200)
		f.member = w.Result().Cookies()[0]
	})
	t.Run("password invalidates every session", func(t *testing.T) {
		w := f.json("POST", "/api/v1/auth/login", nil, `{"username":"member","password":"correct-password-123"}`, 200)
		second := w.Result().Cookies()[0]
		f.json("POST", "/api/v1/me/password", f.member, `{"current_password":"wrong-password-12","new_password":"next-password-123"}`, 400)
		f.json("POST", "/api/v1/me/password", f.member, `{"current_password":"correct-password-123","new_password":"next-password-123"}`, 204)
		f.json("GET", "/api/v1/me", second, "", 401)
		f.json("GET", "/api/v1/me", f.member, "", 401)
	})
	t.Run("deletion and logout", func(t *testing.T) {
		f.json("DELETE", "/api/v1/posts/"+p.ID, f.owner, "", 204)
		f.json("GET", p.Images[0].URL, f.owner, "", 404)
		f.json("POST", "/api/v1/auth/logout", f.owner, "", 204)
		f.json("GET", "/api/v1/me", f.owner, "", 401)
	})
}
func TestRateLimitAndExpiry(t *testing.T) {
	f := setup(t)
	f.db.Exec(context.Background(), `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE user_id=$1`, f.memberID)
	f.json("GET", "/api/v1/me", f.member, "", 401)
	for i := 0; i < 15; i++ {
		w := f.req("POST", "/api/v1/auth/login", nil, strings.NewReader(`{"username":"missing","password":"correct-password-123"}`), "application/json", f.s.Config.Origin)
		if w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	f.json("POST", "/api/v1/auth/login", nil, `{"username":"missing","password":"correct-password-123"}`, 429)
}
func TestLimiterBoundAndReset(t *testing.T) {
	l := &rateLimiter{entries: map[string]rateEntry{}}
	if !l.allow("x", 1, time.Hour) || l.allow("x", 1, time.Hour) {
		t.Fatal("limit")
	}
	l.entries["x"] = rateEntry{until: time.Now().Add(-time.Second)}
	if !l.allow("x", 1, time.Hour) {
		t.Fatal("expiry")
	}
}
