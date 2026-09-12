-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9_]{3,32}$'),
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 80),
    password_hash text NOT NULL,
    role text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_single_owner ON users (role) WHERE role = 'owner';
CREATE TABLE invites (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash text NOT NULL UNIQUE,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    expires_at timestamptz NOT NULL,
    used_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((used_at IS NULL) = (used_by IS NULL))
);
CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_id ON sessions(user_id);
CREATE INDEX sessions_expires_at ON sessions(expires_at);
CREATE TABLE posts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    body text NOT NULL CHECK (char_length(body) <= 5000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX posts_feed ON posts(created_at DESC, id DESC);
CREATE TABLE post_images (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id uuid NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    mime_type text NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png')),
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    width integer NOT NULL CHECK (width > 0),
    height integer NOT NULL CHECK (height > 0),
    position integer NOT NULL CHECK (position BETWEEN 0 AND 3),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(post_id, position)
);

-- +goose Down
DROP TABLE post_images;
DROP TABLE posts;
DROP TABLE sessions;
DROP TABLE invites;
DROP TABLE users;
