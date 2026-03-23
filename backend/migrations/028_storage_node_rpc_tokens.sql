ALTER TABLE storage_nodes
    ADD COLUMN IF NOT EXISTS rpc_token_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rpc_token_ciphertext TEXT NOT NULL DEFAULT '';
