CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE charge_status AS ENUM (
    'pending',
    'processing',
    'boleto_generated',
    'paid',
    'failed',
    'cancelled'
);

CREATE TABLE charges (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    charge_id       VARCHAR(100) UNIQUE NOT NULL,
    customer_id     VARCHAR(100) NOT NULL,
    customer_name   VARCHAR(255) NOT NULL,
    customer_doc    VARCHAR(14) NOT NULL,
    amount_cents    BIGINT NOT NULL,
    due_date        DATE NOT NULL,
    description     TEXT,
    status          charge_status NOT NULL DEFAULT 'pending',
    boleto_url      TEXT,
    boleto_barcode  VARCHAR(60),
    attempts        INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 5,
    last_error      TEXT,
    metadata        JSONB,
    idempotency_key VARCHAR(100) UNIQUE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_charges_status    ON charges(status);
CREATE INDEX idx_charges_customer  ON charges(customer_id);
CREATE INDEX idx_charges_due_date  ON charges(due_date);
