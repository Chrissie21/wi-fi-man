-- name: CreateToken :one
INSERT INTO tokens (id, plan_id, code_hash, status, created_at, updated_at, activated_at, expires_at, used_by_device_mac, payment_id, notes, sold_by_agent_id, revoked_reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetTokenByCodeHash :one
SELECT * FROM tokens WHERE code_hash = $1;

-- name: UpdateTokenState :exec
UPDATE tokens
SET status = $2,
    updated_at = $3,
    activated_at = $4,
    expires_at = $5,
    used_by_device_mac = $6,
    revoked_reason = $7
WHERE id = $1;
