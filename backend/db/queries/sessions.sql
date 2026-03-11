-- name: CreateSession :one
INSERT INTO sessions (id, token_id, device_mac, ip_address, gateway_id, started_at, ended_at, bytes_up, bytes_down, status, end_reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetActiveSessionByToken :one
SELECT * FROM sessions WHERE token_id = $1 AND status = 'active' LIMIT 1;

-- name: UpdateSessionAccounting :exec
UPDATE sessions
SET bytes_up = $2,
    bytes_down = $3,
    status = $4,
    ended_at = $5,
    end_reason = $6
WHERE id = $1;
