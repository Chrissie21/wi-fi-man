-- name: CreatePlan :one
INSERT INTO plans (id, name, duration_minutes, data_limit_mb, speed_down_kbps, speed_up_kbps, device_limit, price, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListPlans :many
SELECT * FROM plans ORDER BY created_at;
