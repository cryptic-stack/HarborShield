CREATE TABLE IF NOT EXISTS storage_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    endpoint TEXT NOT NULL UNIQUE,
    backend_type TEXT NOT NULL DEFAULT 'distributed',
    zone TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    capacity_bytes BIGINT NOT NULL DEFAULT 0,
    used_bytes BIGINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS object_placements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_id UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    replica_index INTEGER NOT NULL DEFAULT 0,
    chunk_ordinal INTEGER NOT NULL DEFAULT 0,
    storage_node_id UUID REFERENCES storage_nodes(id) ON DELETE SET NULL,
    locator TEXT NOT NULL,
    checksum_sha256 TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'planned',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (object_id, replica_index, chunk_ordinal)
);

CREATE INDEX IF NOT EXISTS object_placements_object_idx
    ON object_placements (object_id, replica_index, chunk_ordinal);

CREATE INDEX IF NOT EXISTS object_placements_node_idx
    ON object_placements (storage_node_id, state);
