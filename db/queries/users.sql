-- name: CreateUser :one
INSERT INTO users (email, name, picture, role, oidc_subject)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByOIDCSubject :one
SELECT * FROM users WHERE oidc_subject = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;

-- name: UpdateUser :one
UPDATE users SET
    email = COALESCE(sqlc.narg('email'), email),
    name = COALESCE(sqlc.narg('name'), name),
    picture = COALESCE(sqlc.narg('picture'), picture),
    role = COALESCE(sqlc.narg('role'), role),
    oidc_subject = COALESCE(sqlc.narg('oidc_subject'), oidc_subject),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;
