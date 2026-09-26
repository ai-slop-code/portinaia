-- name: CreateAdministrator :one
INSERT INTO administrators (
    id, username, password_hash, password_algorithm, password_parameters,
    created_at, updated_at
) VALUES (?, ?, ?, 'argon2id', ?, ?, ?)
RETURNING *;

-- name: GetEnabledAdministrator :one
SELECT *
FROM administrators
WHERE disabled_at IS NULL
LIMIT 1;

-- name: GetAdministratorByUsername :one
SELECT *
FROM administrators
WHERE username = ? COLLATE NOCASE
LIMIT 1;

-- name: UpdateAdministratorPassword :execrows
UPDATE administrators
SET password_hash = ?, password_parameters = ?, updated_at = ?
WHERE id = ? AND disabled_at IS NULL;
