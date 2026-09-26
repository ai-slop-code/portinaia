-- name: CreateProvider :one
INSERT INTO providers (
    id, name, website, external_account_reference, description, notes,
    revision, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
RETURNING *;

-- name: GetProvider :one
SELECT * FROM providers WHERE id = ?;

-- name: ListProviders :many
SELECT *
FROM providers
WHERE (CAST(sqlc.arg(include_archived) AS INTEGER) = 1 OR archived_at IS NULL)
  AND (CAST(sqlc.arg(search) AS TEXT) = '' OR name LIKE '%' || CAST(sqlc.arg(search) AS TEXT) || '%' COLLATE NOCASE)
  AND (name > CAST(sqlc.arg(after_name) AS TEXT)
       OR (name = CAST(sqlc.arg(after_name) AS TEXT) AND id > CAST(sqlc.arg(after_id) AS TEXT)))
ORDER BY name, id
LIMIT CAST(sqlc.arg(page_size) AS INTEGER);
