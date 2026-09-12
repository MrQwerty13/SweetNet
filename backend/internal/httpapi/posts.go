package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"sweetnet/internal/media"
)

type Author struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
type PostImage struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Post struct {
	ID        string      `json:"id"`
	Author    Author      `json:"author"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Images    []PostImage `json:"images"`
}

const postColumns = `p.id::text,u.id::text,u.username,u.display_name,p.body,p.created_at,p.updated_at`

func scanPost(row pgx.Row) (p Post, err error) {
	p.Images = []PostImage{}
	err = row.Scan(&p.ID, &p.Author.ID, &p.Author.Username, &p.Author.DisplayName, &p.Body, &p.CreatedAt, &p.UpdatedAt)
	return
}
func validBody(body string) bool {
	return utf8.ValidString(body) && utf8.RuneCountInString(body) <= 5000
}
func (s *Server) post(ctx context.Context, id string) (Post, error) {
	p, err := scanPost(s.DB.QueryRow(ctx, `SELECT `+postColumns+` FROM posts p JOIN users u ON u.id=p.author_id WHERE p.id=$1`, id))
	if err != nil {
		return p, err
	}
	posts := []Post{p}
	err = s.images(ctx, posts)
	return posts[0], err
}
func (s *Server) images(ctx context.Context, posts []Post) error {
	if len(posts) == 0 {
		return nil
	}
	ids := make([]string, len(posts))
	indices := map[string]int{}
	for i, p := range posts {
		ids[i] = p.ID
		indices[p.ID] = i
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,post_id::text,width,height FROM post_images WHERE post_id=ANY($1::uuid[]) ORDER BY position`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var img PostImage
		var postID string
		if err = rows.Scan(&img.ID, &postID, &img.Width, &img.Height); err != nil {
			return err
		}
		img.URL = "/api/v1/media/" + img.ID
		idx := indices[postID]
		posts[idx].Images = append(posts[idx].Images, img)
	}
	return rows.Err()
}
func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	p, err := s.post(r.Context(), id)
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, p)
}

type cursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		v, err := strconv.Atoi(value)
		if err != nil || v < 1 || v > 50 {
			fail(w, 400, "VALIDATION_ERROR", "Размер страницы: 1–50")
			return
		}
		limit = v
	}
	c := cursor{At: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 512 || err != nil || json.Unmarshal(data, &c) != nil || c.At.IsZero() {
			fail(w, 400, "INVALID_CURSOR", "Некорректная страница")
			return
		}
		if _, err := uuid.Parse(c.ID); err != nil {
			fail(w, 400, "INVALID_CURSOR", "Некорректная страница")
			return
		}
	}
	rows, err := s.DB.Query(r.Context(), `SELECT `+postColumns+` FROM posts p JOIN users u ON u.id=p.author_id WHERE (p.created_at,p.id)<($1,$2::uuid) ORDER BY p.created_at DESC,p.id DESC LIMIT $3`, c.At, c.ID, limit+1)
	if err != nil {
		dbError(w, err)
		return
	}
	items := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		items = append(items, p)
	}
	rows.Close()
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	var next *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		data, _ := json.Marshal(cursor{At: last.CreatedAt, ID: last.ID})
		str := base64.RawURLEncoding.EncodeToString(data)
		next = &str
	}
	if err = s.images(r.Context(), items); err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, map[string]any{"items": items, "next_cursor": next})
}
func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, "upload:"+current(r).ID, 30, time.Hour) {
		return
	}
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		fail(w, 429, "BUSY", "Подождите завершения других загрузок")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 42*1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			fail(w, 413, "TOO_LARGE", "Суммарный размер запроса превышает 42 MiB")
		} else {
			fail(w, 400, "VALIDATION_ERROR", "Ожидается корректная multipart-форма")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	for key := range r.MultipartForm.Value {
		if key != "body" {
			fail(w, 400, "VALIDATION_ERROR", "Неизвестное поле формы")
			return
		}
	}
	for key := range r.MultipartForm.File {
		if key != "images" {
			fail(w, 400, "VALIDATION_ERROR", "Неизвестное поле загрузки")
			return
		}
	}
	if len(r.MultipartForm.Value["body"]) > 1 {
		fail(w, 400, "VALIDATION_ERROR", "Текст должен быть передан один раз")
		return
	}
	body := r.FormValue("body")
	files := r.MultipartForm.File["images"]
	if !validBody(body) || len(files) > 4 || (strings.TrimSpace(body) == "" && len(files) == 0) {
		fail(w, 400, "VALIDATION_ERROR", "Добавьте текст до 5000 символов или до 4 фотографий")
		return
	}
	saved := []media.File{}
	committed := false
	defer func() {
		if !committed {
			for _, f := range saved {
				if err := os.Remove(filepath.Join(s.Config.UploadDir, f.Key)); err != nil && !os.IsNotExist(err) {
					slog.Error("upload rollback cleanup failed")
				}
			}
		}
	}()
	for _, header := range files {
		if header.Size > media.MaxBytes {
			fail(w, 413, "TOO_LARGE", "Фотография превышает 10 MiB")
			return
		}
		f, err := header.Open()
		if err != nil {
			dbError(w, err)
			return
		}
		processed, err := media.Save(s.Config.UploadDir, f, header.Header.Get("Content-Type"))
		f.Close()
		if errors.Is(err, media.ErrLarge) {
			fail(w, 413, "TOO_LARGE", "Фото превышает 10 MiB или 25 мегапикселей")
			return
		}
		if errors.Is(err, media.ErrFormat) {
			fail(w, 415, "UNSUPPORTED_TYPE", "Разрешены только настоящие JPEG и PNG")
			return
		}
		if err != nil {
			dbError(w, err)
			return
		}
		saved = append(saved, processed)
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Recheck membership after potentially slow image processing; lock against disable.
	var active bool
	if err = tx.QueryRow(r.Context(), `SELECT is_active FROM users WHERE id=$1 FOR SHARE`, current(r).ID).Scan(&active); err != nil {
		dbError(w, err)
		return
	}
	if !active {
		fail(w, 401, "UNAUTHENTICATED", "Аккаунт отключён")
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO posts(author_id,body) VALUES($1,$2) RETURNING id::text`, current(r).ID, body).Scan(&id)
	if err != nil {
		dbError(w, err)
		return
	}
	for i, f := range saved {
		_, err = tx.Exec(r.Context(), `INSERT INTO post_images(post_id,storage_key,mime_type,byte_size,width,height,position) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, f.Key, f.MIME, f.Size, f.Width, f.Height, i)
		if err != nil {
			dbError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	committed = true
	p, err := s.post(r.Context(), id)
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, 201, p)
}
func (s *Server) editPost(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validBody(in.Body) {
		fail(w, 400, "VALIDATION_ERROR", "Текст длиннее 5000 символов")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var authorID string
	if err = tx.QueryRow(r.Context(), `SELECT author_id::text FROM posts WHERE id=$1 FOR UPDATE`, id).Scan(&authorID); err != nil {
		dbError(w, err)
		return
	}
	if authorID != current(r).ID {
		fail(w, 403, "FORBIDDEN", "Редактировать можно только свои посты")
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM post_images WHERE post_id=$1`, id).Scan(&count); err != nil {
			dbError(w, err)
			return
		}
		if count == 0 {
			fail(w, 400, "VALIDATION_ERROR", "Пост не может быть пустым")
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE posts SET body=$1,updated_at=now() WHERE id=$2`, in.Body, id); err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	p, err := s.post(r.Context(), id)
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, p)
}
func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var authorID string
	if err = tx.QueryRow(r.Context(), `SELECT author_id::text FROM posts WHERE id=$1 FOR UPDATE`, id).Scan(&authorID); err != nil {
		dbError(w, err)
		return
	}
	if authorID != current(r).ID && current(r).Role != "owner" {
		fail(w, 403, "FORBIDDEN", "Удалять можно только свои посты")
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT storage_key FROM post_images WHERE post_id=$1`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		keys = append(keys, key)
	}
	rows.Close()
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM posts WHERE id=$1`, id); err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	for _, key := range keys {
		if err := os.Remove(filepath.Join(s.Config.UploadDir, key)); err != nil && !os.IsNotExist(err) {
			slog.Error("deleted media needs offline reconciliation")
		}
	}
	w.WriteHeader(204)
}
func (s *Server) getMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var key, mime string
	err := s.DB.QueryRow(r.Context(), `SELECT i.storage_key,i.mime_type FROM post_images i JOIN posts p ON p.id=i.post_id WHERE i.id=$1`, id).Scan(&key, &mime)
	if err != nil {
		dbError(w, err)
		return
	}
	file, err := os.Open(filepath.Join(s.Config.UploadDir, key))
	if err != nil {
		fail(w, 404, "NOT_FOUND", "Фотография не найдена")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		dbError(w, err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, "image", info.ModTime(), file)
}
