-- Таблица для хранения платежей
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    amount DECIMAL(10,2) NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    sender_id VARCHAR(100) NOT NULL,
    receiver_id VARCHAR(100) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Индекс для быстрого поиска по статусу
CREATE INDEX idx_payments_status ON payments(status);

-- Индекс для поиска по отправителю
CREATE INDEX idx_payments_sender_id ON payments(sender_id);

-- Таблица для идемпотентности
CREATE TABLE IF NOT EXISTS idempotency_keys (
    id SERIAL PRIMARY KEY,
    idempotency_key VARCHAR(255) NOT NULL,
    owner_id VARCHAR(100) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'in_progress',
    response_status_code INTEGER,
    response_body JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(owner_id, idempotency_key)
);

-- Индекс для быстрого поиска по ключу
CREATE INDEX idx_idempotency_keys_lookup ON idempotency_keys(owner_id, idempotency_key);

-- Индекс для очистки старых записей
CREATE INDEX idx_idempotency_keys_created_at ON idempotency_keys(created_at);
