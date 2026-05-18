CREATE UNIQUE INDEX IF NOT EXISTS idx_repositories_name_unique ON repositories(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repositories_type_endpoint_unique ON repositories(type, endpoint);
