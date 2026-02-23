-- name: GetDefaults :one
SELECT * FROM instance_defaults LIMIT 1;

-- name: UpdateDefaults :one
UPDATE instance_defaults SET
    image = $1,
    cpu_request = $2,
    memory_request = $3,
    cpu_limit = $4,
    memory_limit = $5,
    storage_size = $6,
    ingress_domain = $7,
    updated_at = now()
RETURNING *;
