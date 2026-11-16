-- name: CreateProvider :one
INSERT INTO providers (
    id, name, website, external_account_reference, description, notes,
    revision, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
RETURNING *;

-- name: GetProvider :one
SELECT * FROM providers WHERE id = ?;

-- name: UpdateProvider :execrows
UPDATE providers
SET name = ?, website = ?, external_account_reference = ?, description = ?, notes = ?,
    revision = ?, updated_at = ?
WHERE id = ? AND revision = ? AND archived_at IS NULL;

-- name: DeleteProviderRoles :exec
DELETE FROM provider_roles WHERE provider_id = ?;

-- name: InsertProviderRole :exec
INSERT INTO provider_roles (provider_id, role) VALUES (?, ?);

-- name: ListProviderRoles :many
SELECT role FROM provider_roles WHERE provider_id = ? ORDER BY role;

-- name: ListProviders :many
SELECT *
FROM providers
WHERE (
    CAST(sqlc.arg(archived_mode) AS TEXT) = 'include'
    OR (CAST(sqlc.arg(archived_mode) AS TEXT) = 'exclude' AND archived_at IS NULL)
    OR (CAST(sqlc.arg(archived_mode) AS TEXT) = 'only' AND archived_at IS NOT NULL)
)
  AND (CAST(sqlc.arg(search) AS TEXT) = '' OR name LIKE '%' || CAST(sqlc.arg(search) AS TEXT) || '%' COLLATE NOCASE)
  AND (name > CAST(sqlc.arg(after_name) AS TEXT)
       OR (name = CAST(sqlc.arg(after_name) AS TEXT) AND id > CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY name, id
LIMIT CAST(sqlc.arg(page_size) AS INTEGER);
