ALTER TABLE orders ADD COLUMN polled_at timestamptz NULL;

DROP INDEX orders_pending_idx;
CREATE INDEX orders_pending_idx ON orders (polled_at NULLS FIRST, uploaded_at)
    WHERE status IN ('NEW', 'PROCESSING');
