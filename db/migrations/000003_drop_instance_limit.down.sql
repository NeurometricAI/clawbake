ALTER TABLE users ADD COLUMN instance_limit INTEGER NOT NULL DEFAULT 1;
ALTER TABLE instance_defaults ADD COLUMN ingress_domain TEXT NOT NULL DEFAULT 'claw.example.com';
