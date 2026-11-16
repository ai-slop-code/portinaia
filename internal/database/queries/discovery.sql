-- name: CreateAgent :one
INSERT INTO agents (
    id, compute_id, credential_hash, credential_generation, protocol_version,
    client_version, enrolled_at, revision
) VALUES (?, ?, ?, 1, ?, ?, ?, 1)
RETURNING *;

-- name: GetAgentByCredentialHash :one
SELECT * FROM agents WHERE credential_hash = ? AND revoked_at IS NULL;

-- name: ListAgents :many
SELECT agents.*, computes.display_name
FROM agents
JOIN computes ON computes.id = agents.compute_id
WHERE (
    CAST(sqlc.narg(after_heartbeat) AS TEXT) IS NULL
    AND (
        agents.last_heartbeat_at IS NOT NULL
        OR (agents.last_heartbeat_at IS NULL AND agents.id > CAST(sqlc.arg(after_id) AS TEXT))
    )
) OR (
    CAST(sqlc.narg(after_heartbeat) AS TEXT) IS NOT NULL
    AND agents.last_heartbeat_at IS NOT NULL
    AND (
        agents.last_heartbeat_at > CAST(sqlc.narg(after_heartbeat) AS TEXT)
        OR (
            agents.last_heartbeat_at = CAST(sqlc.narg(after_heartbeat) AS TEXT)
            AND agents.id > CAST(sqlc.arg(after_id) AS TEXT)
        )
    )
)
ORDER BY agents.last_heartbeat_at IS NOT NULL, agents.last_heartbeat_at, agents.id
LIMIT CAST(sqlc.arg(page_size) AS INTEGER);

-- name: PutSnapshotContent :one
INSERT INTO snapshot_contents (
    id, format_version, content_hash, compressed_document, uncompressed_size, created_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (format_version, content_hash) DO UPDATE SET content_hash = excluded.content_hash
RETURNING *;

-- name: CreateDiscoveryReport :one
INSERT INTO discovery_reports (
    id, report_id, agent_id, protocol_version, received_at, client_collected_at,
    snapshot_content_id, result, collection_error_summary
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDiscoveryReportByAgentReportID :one
SELECT * FROM discovery_reports WHERE agent_id = ? AND report_id = ?;

-- name: ListAgentReports :many
SELECT *
FROM discovery_reports
WHERE agent_id = ?
  AND (received_at < sqlc.arg(before_received_at)
       OR (received_at = sqlc.arg(before_received_at) AND id < sqlc.arg(before_id)))
ORDER BY received_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: CountExpiredDiscoveryReports :one
SELECT count(*) FROM discovery_reports WHERE received_at < sqlc.arg(cutoff);

-- name: CountReclaimableSnapshotContents :one
SELECT count(*)
FROM snapshot_contents content
WHERE NOT EXISTS (
    SELECT 1 FROM discovery_reports report
    WHERE report.snapshot_content_id = content.id
      AND report.received_at >= sqlc.arg(cutoff)
);

-- name: DeleteExpiredDiscoveryReportsBatch :execrows
DELETE FROM discovery_reports
WHERE id IN (
    SELECT candidate.id FROM discovery_reports candidate
    WHERE candidate.received_at < sqlc.arg(cutoff)
    ORDER BY candidate.received_at, candidate.id
    LIMIT sqlc.arg(batch_size)
);

-- name: DeleteUnreferencedSnapshotContentsBatch :execrows
DELETE FROM snapshot_contents
WHERE id IN (
    SELECT content.id FROM snapshot_contents content
    WHERE NOT EXISTS (
        SELECT 1 FROM discovery_reports report
        WHERE report.snapshot_content_id = content.id
    )
    ORDER BY content.created_at, content.id
    LIMIT sqlc.arg(batch_size)
);
