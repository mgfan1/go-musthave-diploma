DROP INDEX orders_pending_idx;
CREATE INDEX orders_pending_idx ON orders (uploaded_at) WHERE status IN ('NEW', 'PROCESSING');

ALTER TABLE orders DROP COLUMN polled_at;
