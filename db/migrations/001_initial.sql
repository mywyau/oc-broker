CREATE TABLE IF NOT EXISTS claimants (
    id UUID PRIMARY KEY,
    full_name TEXT NOT NULL CHECK (length(trim(full_name)) >= 2),
    email TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS claimants_email_unique ON claimants (lower(email));

CREATE TABLE IF NOT EXISTS claims (
    id UUID PRIMARY KEY,
    claimant_id UUID NOT NULL REFERENCES claimants(id),
    policy_number TEXT NOT NULL,
    incident_date DATE NOT NULL,
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    currency CHAR(3) NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'REVIEW_REQUIRED', 'REJECTED')),
    decision_reason TEXT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS claims_claimant_created_idx ON claims (claimant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS outbox_events (
    event_id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    event_version INTEGER NOT NULL,
    aggregate_id UUID NOT NULL,
    target_queue TEXT NOT NULL CHECK (target_queue IN ('claim-commands', 'claim-events')),
    correlation_id UUID NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT
);
CREATE INDEX IF NOT EXISTS outbox_unpublished_idx ON outbox_events (occurred_at) WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS inbox_messages (
    event_id UUID PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
