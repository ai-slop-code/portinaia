-- name: GetDNSProjection :one
SELECT * FROM dns_projections WHERE id = ?;

-- name: ListDNSProjectionsForName :many
SELECT * FROM dns_projections WHERE dns_name_id = ? ORDER BY provider;

-- name: ScheduleDNSProjection :one
INSERT INTO dns_projections (
    id, dns_name_id, provider, desired_revision, applied_revision, state,
    attempt_count, next_retry_at, updated_at
) VALUES (?, ?, ?, ?, 0, ?, 0, ?, ?)
ON CONFLICT (dns_name_id, provider) DO UPDATE SET
    desired_revision = excluded.desired_revision,
    state = excluded.state,
    attempt_count = 0,
    next_retry_at = excluded.next_retry_at,
    lease_owner = NULL,
    lease_expires_at = NULL,
    leased_revision = NULL,
    updated_at = excluded.updated_at
WHERE excluded.desired_revision > dns_projections.desired_revision
  AND excluded.desired_revision >= dns_projections.applied_revision
RETURNING *;

-- name: ClaimDNSProjections :many
UPDATE dns_projections
SET lease_owner = CAST(sqlc.arg(lease_owner) AS TEXT),
    lease_expires_at = CAST(sqlc.arg(lease_expires_at) AS TEXT),
    leased_revision = desired_revision,
    attempt_count = attempt_count + 1,
    last_attempt_at = CAST(sqlc.arg(claimed_at) AS TEXT),
    updated_at = CAST(sqlc.arg(claimed_at) AS TEXT)
WHERE id IN (
    SELECT id
    FROM dns_projections
    WHERE state IN ('pending', 'failed', 'deleting')
      AND (next_retry_at IS NULL OR next_retry_at <= CAST(sqlc.arg(claimed_at) AS TEXT))
      AND (lease_owner IS NULL OR lease_expires_at <= CAST(sqlc.arg(claimed_at) AS TEXT))
      AND EXISTS (
          SELECT 1 FROM dns_names
          WHERE dns_names.id = dns_projections.dns_name_id
            AND dns_names.desired_revision = dns_projections.desired_revision
      )
    ORDER BY COALESCE(next_retry_at, updated_at), id
    LIMIT CAST(sqlc.arg(batch_size) AS INTEGER)
)
RETURNING *;

-- name: CompleteDNSProjection :execrows
UPDATE dns_projections
SET applied_revision = CAST(sqlc.arg(applied_revision) AS INTEGER),
    state = 'synchronized',
    last_success_at = CAST(sqlc.arg(completed_at) AS TEXT),
    next_retry_at = NULL,
    last_error_code = NULL,
    last_error_message = NULL,
    observed_checksum = sqlc.narg(observed_checksum),
    lease_owner = NULL,
    lease_expires_at = NULL,
    leased_revision = NULL,
    updated_at = CAST(sqlc.arg(completed_at) AS TEXT)
WHERE id = CAST(sqlc.arg(id) AS TEXT)
  AND lease_owner = CAST(sqlc.arg(lease_owner) AS TEXT)
  AND lease_expires_at > CAST(sqlc.arg(completed_at) AS TEXT)
  AND leased_revision = CAST(sqlc.arg(applied_revision) AS INTEGER)
  AND desired_revision = CAST(sqlc.arg(applied_revision) AS INTEGER)
  AND EXISTS (
      SELECT 1 FROM dns_names
      WHERE dns_names.id = dns_projections.dns_name_id
        AND dns_names.desired_revision = CAST(sqlc.arg(applied_revision) AS INTEGER)
  );

-- name: FailDNSProjection :execrows
UPDATE dns_projections
SET state = 'failed',
    next_retry_at = CAST(sqlc.arg(next_retry_at) AS TEXT),
    last_error_code = CAST(sqlc.arg(error_code) AS TEXT),
    last_error_message = CAST(sqlc.arg(error_message) AS TEXT),
    lease_owner = NULL,
    lease_expires_at = NULL,
    leased_revision = NULL,
    updated_at = CAST(sqlc.arg(failed_at) AS TEXT)
WHERE id = CAST(sqlc.arg(id) AS TEXT)
  AND lease_owner = CAST(sqlc.arg(lease_owner) AS TEXT)
  AND lease_expires_at > CAST(sqlc.arg(failed_at) AS TEXT)
  AND leased_revision = CAST(sqlc.arg(failed_revision) AS INTEGER)
  AND desired_revision = CAST(sqlc.arg(failed_revision) AS INTEGER)
  AND EXISTS (
      SELECT 1 FROM dns_names
      WHERE dns_names.id = dns_projections.dns_name_id
        AND dns_names.desired_revision = CAST(sqlc.arg(failed_revision) AS INTEGER)
  );

-- name: ReleaseExpiredDNSProjectionLeases :execrows
UPDATE dns_projections
SET lease_owner = NULL, lease_expires_at = NULL, leased_revision = NULL, updated_at = ?
WHERE lease_owner IS NOT NULL AND lease_expires_at <= ?;
