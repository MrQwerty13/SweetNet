package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"sweetnet/internal/auth"
)

type registration struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, "register-ip:"+ip(r), 30, time.Hour) {
		return
	}
	var in registration
	if !decode(w, r, &in) {
		return
	}
	username, valid := auth.Username(in.Username)
	if !s.limit(w, "register-user:"+username, 10, time.Hour) {
		return
	}
	if !valid || !auth.ValidName(in.DisplayName) || !auth.ValidPassword(in.Password) || len(in.Token) != 43 {
		fail(w, 400, "VALIDATION_ERROR", "Проверьте приглашение, имя и пароль (12–128 символов)")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		dbError(w, err)
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var inviteID string
	err = tx.QueryRow(r.Context(), `SELECT id::text FROM invites WHERE token_hash=$1 AND expires_at>now() AND used_at IS NULL AND revoked_at IS NULL FOR UPDATE`, auth.HashSecret(in.Token)).Scan(&inviteID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 400, "INVALID_INVITE", "Приглашение недействительно или уже использовано")
		return
	}
	if err != nil {
		dbError(w, err)
		return
	}
	u, err := scanUser(tx.QueryRow(r.Context(), `INSERT INTO users(username,display_name,password_hash) VALUES($1,$2,$3) RETURNING `+userColumns, username, in.DisplayName, hash))
	if err != nil {
		dbError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE invites SET used_at=now(),used_by=$1 WHERE id=$2`, u.ID, inviteID); err != nil {
		dbError(w, err)
		return
	}
	secret, err := newSession(r.Context(), tx, u.ID)
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	s.setCookie(w, secret)
	write(w, 201, u)
}
func newSession(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	secret, err := auth.Secret()
	if err != nil {
		return "", err
	}
	// Bound session storage for a small community; retain at most ten per user.
	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND (expires_at<=now() OR id IN (SELECT id FROM sessions WHERE user_id=$1 ORDER BY created_at DESC OFFSET 9))`, userID)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, userID, auth.HashSecret(secret), time.Now().Add(SessionLifetime))
	return secret, err
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, "login-ip:"+ip(r), 100, 15*time.Minute) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	username, valid := auth.Username(in.Username)
	if !s.limit(w, "login-user:"+username, 15, 15*time.Minute) {
		return
	}
	if !valid || !auth.ValidPassword(in.Password) {
		fail(w, 401, "INVALID_CREDENTIALS", "Неверное имя пользователя или пароль")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var hash string
	var u User
	err = tx.QueryRow(r.Context(), `SELECT `+userColumns+`, password_hash FROM users WHERE username=$1 FOR UPDATE`, username).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Active, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		auth.CheckPassword(in.Password, s.dummyHash)
		fail(w, 401, "INVALID_CREDENTIALS", "Неверное имя пользователя или пароль")
		return
	}
	if err != nil {
		dbError(w, err)
		return
	}
	matches := auth.CheckPassword(in.Password, hash)
	if !matches || !u.Active {
		fail(w, 401, "INVALID_CREDENTIALS", "Неверное имя пользователя или пароль")
		return
	}
	secret, err := newSession(r.Context(), tx, u.ID)
	if err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	s.setCookie(w, secret)
	write(w, 200, u)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(cookieName)
	if _, err := s.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, auth.HashSecret(c.Value)); err != nil {
		dbError(w, err)
		return
	}
	s.clearCookie(w)
	w.WriteHeader(204)
}
func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !auth.ValidName(in.DisplayName) {
		fail(w, 400, "VALIDATION_ERROR", "Имя должно содержать от 1 до 80 символов")
		return
	}
	u, err := scanUser(s.DB.QueryRow(r.Context(), `UPDATE users SET display_name=$1,updated_at=now() WHERE id=$2 AND is_active RETURNING `+userColumns, in.DisplayName, current(r).ID))
	if err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, u)
}
func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, "password:"+current(r).ID, 10, 15*time.Minute) {
		return
	}
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !auth.ValidPassword(in.New) || !auth.ValidPassword(in.Current) {
		fail(w, 400, "VALIDATION_ERROR", "Пароль должен содержать 12–128 символов")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var hash string
	if err = tx.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1 AND is_active FOR UPDATE`, current(r).ID).Scan(&hash); err != nil {
		dbError(w, err)
		return
	}
	if !auth.CheckPassword(in.Current, hash) {
		fail(w, 400, "INVALID_PASSWORD", "Текущий пароль неверен")
		return
	}
	hash, err = auth.HashPassword(in.New)
	if err != nil {
		dbError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE users SET password_hash=$1,updated_at=now() WHERE id=$2`, hash, current(r).ID); err != nil {
		dbError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, current(r).ID); err != nil {
		dbError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	s.clearCookie(w)
	w.WriteHeader(204)
}

type Invite struct {
	ID        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	URL       string     `json:"url,omitempty"`
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	if !s.limit(w, "invite:"+current(r).ID, 30, time.Hour) {
		return
	}
	secret, err := auth.Secret()
	if err != nil {
		dbError(w, err)
		return
	}
	var inv Invite
	err = s.DB.QueryRow(r.Context(), `INSERT INTO invites(token_hash,created_by,expires_at) VALUES($1,$2,now()+interval '7 days') RETURNING id::text,created_at,expires_at`, auth.HashSecret(secret), current(r).ID).Scan(&inv.ID, &inv.CreatedAt, &inv.ExpiresAt)
	if err != nil {
		dbError(w, err)
		return
	}
	inv.URL = s.Config.Origin + "/join#token=" + secret
	write(w, 201, inv)
}
func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT id::text,created_at,expires_at,used_at,revoked_at FROM invites ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []Invite{}
	for rows.Next() {
		var i Invite
		if err = rows.Scan(&i.ID, &i.CreatedAt, &i.ExpiresAt, &i.UsedAt, &i.RevokedAt); err != nil {
			dbError(w, err)
			return
		}
		items = append(items, i)
	}
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	write(w, 200, map[string]any{"items": items})
}
func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE invites SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND used_at IS NULL`, id)
	if err != nil {
		dbError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "NOT_FOUND", "Приглашение не найдено или уже использовано")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT `+userColumns+` FROM users ORDER BY created_at,id`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	items := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		items = append(items, u)
	}
	if rows.Err() != nil {
		dbError(w, rows.Err())
		return
	}
	write(w, 200, map[string]any{"items": items})
}
func (s *Server) setActive(w http.ResponseWriter, r *http.Request) {
	id, ok := resourceID(w, r)
	if !ok {
		return
	}
	var in struct {
		Active *bool `json:"is_active"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Active == nil {
		fail(w, 400, "VALIDATION_ERROR", "Укажите is_active")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		dbError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	u, err := scanUser(tx.QueryRow(r.Context(), `SELECT `+userColumns+` FROM users WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		dbError(w, err)
		return
	}
	if u.Role == "owner" {
		fail(w, 403, "FORBIDDEN", "Нельзя отключить владельца")
		return
	}
	u, err = scanUser(tx.QueryRow(r.Context(), `UPDATE users SET is_active=$1,updated_at=now() WHERE id=$2 RETURNING `+userColumns, *in.Active, id))
	if err != nil {
		dbError(w, err)
		return
	}
	if !*in.Active {
		if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
			dbError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		dbError(w, err)
		return
	}
	write(w, 200, u)
}
