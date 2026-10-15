-- Adiciona o provedor responsável por cada carteira.
ALTER TABLE wallets
    ADD COLUMN provider_id TEXT NOT NULL DEFAULT 'default';

-- Remove a restrição antiga, que não considera o provedor.
ALTER TABLE wallets
    DROP CONSTRAINT wallets_player_id_currency_key;

-- Cada provedor pode ter sua própria carteira por jogador e moeda.
ALTER TABLE wallets
    ADD CONSTRAINT wallets_provider_player_currency_key
    UNIQUE (provider_id, player_id, currency);

-- Índice para consultas e validações por provedor.
CREATE INDEX idx_wallets_provider
    ON wallets(provider_id);
