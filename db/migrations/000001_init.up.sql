CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    picture TEXT DEFAULT '',
    role TEXT NOT NULL DEFAULT 'user',
    oidc_subject TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE instance_defaults (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    image TEXT NOT NULL DEFAULT 'ghcr.io/openclaw/openclaw:latest',
    cpu_request TEXT NOT NULL DEFAULT '100m',
    memory_request TEXT NOT NULL DEFAULT '1Gi',
    cpu_limit TEXT NOT NULL DEFAULT '500m',
    memory_limit TEXT NOT NULL DEFAULT '2Gi',
    storage_size TEXT NOT NULL DEFAULT '5Gi',
    ingress_domain TEXT NOT NULL DEFAULT 'claw.example.com',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure exactly one defaults row
INSERT INTO instance_defaults (id) VALUES (gen_random_uuid());
