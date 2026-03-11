-- name: CreatePayment :one
INSERT INTO payments (id, amount, currency, method, transaction_ref, status, metadata, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetPaymentByRef :one
SELECT * FROM payments WHERE transaction_ref = $1;

-- name: ListPaidPayments :many
SELECT * FROM payments WHERE status = 'paid';
