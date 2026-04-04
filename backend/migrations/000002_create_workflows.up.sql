CREATE TABLE workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    template_id TEXT,
    status TEXT DEFAULT 'draft',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE workflow_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID REFERENCES workflows(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id),
    label TEXT,
    position_x FLOAT,
    position_y FLOAT,
    is_entry BOOLEAN DEFAULT FALSE
);

CREATE TABLE workflow_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID REFERENCES workflows(id) ON DELETE CASCADE,
    source_node_id UUID REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    target_node_id UUID REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    condition TEXT,
    priority INT DEFAULT 0
);
