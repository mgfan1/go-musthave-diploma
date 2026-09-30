CREATE TABLE users (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    login         text        NOT NULL UNIQUE,
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          bigint        GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number      text          NOT NULL UNIQUE,
    user_id     bigint        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status      text          NOT NULL DEFAULT 'NEW'
                              CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED')),
    accrual     numeric(12,2) NULL CHECK (accrual >= 0),
    uploaded_at timestamptz   NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_uploaded_idx ON orders (user_id, uploaded_at DESC);
CREATE INDEX orders_pending_idx ON orders (uploaded_at) WHERE status IN ('NEW', 'PROCESSING');

CREATE TABLE withdrawals (
    id           bigint        GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    order_number text          NOT NULL,
    amount       numeric(12,2) NOT NULL CHECK (amount > 0),
    processed_at timestamptz   NOT NULL DEFAULT now()
);

CREATE INDEX withdrawals_user_processed_idx ON withdrawals (user_id, processed_at DESC);
