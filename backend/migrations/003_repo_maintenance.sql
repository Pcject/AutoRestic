ALTER TABLE repositories ADD COLUMN prune_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE repositories ADD COLUMN prune_cron_expr TEXT NOT NULL DEFAULT '0 3 * * 0';
ALTER TABLE repositories ADD COLUMN prune_args TEXT NOT NULL DEFAULT '[]';
ALTER TABLE repositories ADD COLUMN check_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE repositories ADD COLUMN check_cron_expr TEXT NOT NULL DEFAULT '0 4 1 * *';
ALTER TABLE repositories ADD COLUMN check_args TEXT NOT NULL DEFAULT '["--read-data-subset=10%"]';
