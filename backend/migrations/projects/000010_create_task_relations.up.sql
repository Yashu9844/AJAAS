-- Directed relations between tasks (TK-07). blocks and duplicates read source -> target; related is stored once in canonical id order.
CREATE TABLE task_relations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    source_task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    target_task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    type VARCHAR(12) NOT NULL CHECK (type IN ('blocks', 'related', 'duplicates')),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT task_relations_distinct CHECK (source_task_id <> target_task_id)
);
CREATE UNIQUE INDEX uq_task_relations ON task_relations (source_task_id, target_task_id, type);
CREATE INDEX idx_task_relations_source ON task_relations (tenant_id, source_task_id, type);
CREATE INDEX idx_task_relations_target ON task_relations (tenant_id, target_task_id, type);
