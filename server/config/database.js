const Database = require('better-sqlite3');
const path = require('path');
const fs = require('fs');
const logger = require('../utils/logger');

const dbDir = path.join(__dirname, '../../data');
const dbPath = process.env.DATABASE_PATH || path.join(dbDir, 'autorestic.db');

if (!fs.existsSync(dbDir)) {
  fs.mkdirSync(dbDir, { recursive: true });
}

const db = new Database(dbPath);
db.pragma('journal_mode = WAL');

function initDatabase() {
  db.exec(`
    CREATE TABLE IF NOT EXISTS users (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      username TEXT UNIQUE NOT NULL,
      password TEXT NOT NULL,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS repositories (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT UNIQUE NOT NULL,
      type TEXT NOT NULL,
      url TEXT NOT NULL,
      password TEXT NOT NULL,
      env_vars TEXT,
      initialized INTEGER DEFAULT 0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS backups (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT UNIQUE NOT NULL,
      repository_id INTEGER NOT NULL,
      source_paths TEXT NOT NULL,
      exclude_patterns TEXT,
      tags TEXT,
      options TEXT,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      FOREIGN KEY (repository_id) REFERENCES repositories(id)
    );

    CREATE TABLE IF NOT EXISTS schedules (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      backup_ids TEXT NOT NULL,
      cron_expression TEXT NOT NULL,
      enabled INTEGER DEFAULT 1,
      last_run DATETIME,
      next_run DATETIME,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS operation_logs (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      type TEXT NOT NULL,
      repository_id INTEGER,
      backup_id INTEGER,
      snapshot_id TEXT,
      status TEXT NOT NULL,
      message TEXT,
      duration INTEGER,
      restic_command TEXT,
      restic_stdout TEXT,
      restic_stderr TEXT,
      restic_stdout_formatted TEXT,
      restic_stderr_formatted TEXT,
      started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      completed_at DATETIME,
      FOREIGN KEY (repository_id) REFERENCES repositories(id),
      FOREIGN KEY (backup_id) REFERENCES backups(id)
    );

    CREATE TABLE IF NOT EXISTS tasks (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      type TEXT NOT NULL,
      status TEXT NOT NULL,
      repository_id INTEGER,
      backup_id INTEGER,
      snapshot_id TEXT,
      target_path TEXT,
      config TEXT,
      output TEXT,
      error_message TEXT,
      started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      completed_at DATETIME,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      FOREIGN KEY (repository_id) REFERENCES repositories(id),
      FOREIGN KEY (backup_id) REFERENCES backups(id)
    );

    CREATE TABLE IF NOT EXISTS task_logs (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      task_id INTEGER NOT NULL,
      level TEXT NOT NULL,
      message TEXT NOT NULL,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      FOREIGN KEY (task_id) REFERENCES tasks(id)
    );

    CREATE INDEX IF NOT EXISTS idx_backups_repository ON backups(repository_id);
    CREATE INDEX IF NOT EXISTS idx_schedules_backup ON schedules(backup_ids);
    CREATE INDEX IF NOT EXISTS idx_operation_logs_type ON operation_logs(type);
    CREATE INDEX IF NOT EXISTS idx_operation_logs_status ON operation_logs(status);
    CREATE INDEX IF NOT EXISTS idx_operation_logs_repository ON operation_logs(repository_id);
    CREATE INDEX IF NOT EXISTS idx_operation_logs_backup ON operation_logs(backup_id);
    CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks(type);
    CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
    CREATE INDEX IF NOT EXISTS idx_task_logs_task ON task_logs(task_id);
  `);

  function addColumnIfNotExists(table, column, definition) {
    try {
      const stmt = db.prepare(`PRAGMA table_info(${table})`);
      const columns = stmt.all();
      const columnExists = columns.some(col => col.name === column);
      
      if (!columnExists) {
        db.prepare(`ALTER TABLE ${table} ADD COLUMN ${column} ${definition}`).run();
        logger.info(`Added column ${column} to table ${table}`);
      }
    } catch (error) {
      logger.warn(`Failed to add column ${column} to table ${table}:`, error.message);
    }
  }

  addColumnIfNotExists('operation_logs', 'duration', 'INTEGER');
  addColumnIfNotExists('operation_logs', 'restic_command', 'TEXT');
  addColumnIfNotExists('operation_logs', 'restic_stdout', 'TEXT');
  addColumnIfNotExists('operation_logs', 'restic_stderr', 'TEXT');
  addColumnIfNotExists('operation_logs', 'restic_stdout_formatted', 'TEXT');
  addColumnIfNotExists('operation_logs', 'restic_stderr_formatted', 'TEXT');

  const userCount = db.prepare('SELECT COUNT(*) as count FROM users').get();
  if (userCount.count === 0) {
    const bcrypt = require('bcryptjs');
    const hashedPassword = bcrypt.hashSync('admin123', 10);
    db.prepare('INSERT INTO users (username, password) VALUES (?, ?)').run('admin', hashedPassword);
    logger.info('Default admin user created (username: admin, password: admin123)');
  }

  logger.info('Database initialized successfully');
}

module.exports = { db, initDatabase };
