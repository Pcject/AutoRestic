const express = require('express');
const { db } = require('../config/database');
const authMiddleware = require('../middleware/auth');
const ResticService = require('../services/restic');
const OperationLogService = require('../services/operationLog');
const logger = require('../utils/logger');
const { clearSnapshotsCache } = require('./repository');

const router = express.Router();

router.use(authMiddleware);

router.get('/', (req, res) => {
  try {
    const backups = db.prepare(`
      SELECT b.*, r.name as repository_name, r.type as repository_type
      FROM backups b
      JOIN repositories r ON b.repository_id = r.id
      ORDER BY b.created_at DESC
    `).all();

    const formattedBackups = backups.map(backup => ({
      ...backup,
      source_paths: JSON.parse(backup.source_paths),
      exclude_patterns: backup.exclude_patterns ? JSON.parse(backup.exclude_patterns) : [],
      tags: backup.tags ? JSON.parse(backup.tags) : [],
      options: backup.options ? JSON.parse(backup.options) : {}
    }));

    res.json(formattedBackups);
  } catch (error) {
    logger.error('Get backups error:', error);
    res.status(500).json({ error: 'Failed to get backups' });
  }
});

router.get('/:id', (req, res) => {
  try {
    const backup = db.prepare(`
      SELECT b.*, r.name as repository_name, r.type as repository_type, r.url as repository_url, r.password as repository_password, r.env_vars as repository_env_vars
      FROM backups b
      JOIN repositories r ON b.repository_id = r.id
      WHERE b.id = ?
    `).get(req.params.id);

    if (!backup) {
      return res.status(404).json({ error: 'Backup not found' });
    }

    const formattedBackup = {
      ...backup,
      source_paths: JSON.parse(backup.source_paths),
      exclude_patterns: backup.exclude_patterns ? JSON.parse(backup.exclude_patterns) : [],
      tags: backup.tags ? JSON.parse(backup.tags) : [],
      options: backup.options ? JSON.parse(backup.options) : {},
      env_vars: backup.repository_env_vars ? JSON.parse(backup.repository_env_vars) : {}
    };

    res.json(formattedBackup);
  } catch (error) {
    logger.error('Get backup error:', error);
    res.status(500).json({ error: 'Failed to get backup' });
  }
});

router.post('/', (req, res) => {
  try {
    const { name, repository_id, source_paths, exclude_patterns, tags, options } = req.body;

    if (!name || !repository_id || !source_paths || source_paths.length === 0) {
      return res.status(400).json({ error: 'Name, repository_id, and source_paths are required' });
    }

    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(repository_id);
    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const result = db.prepare(`
      INSERT INTO backups (name, repository_id, source_paths, exclude_patterns, tags, options)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(
      name,
      repository_id,
      JSON.stringify(source_paths),
      exclude_patterns ? JSON.stringify(exclude_patterns) : null,
      tags ? JSON.stringify(tags) : null,
      options ? JSON.stringify(options) : null
    );

    logger.info(`Backup ${name} created successfully`);
    res.status(201).json({ message: 'Backup created successfully', id: result.lastInsertRowid });
  } catch (error) {
    if (error.code === 'SQLITE_CONSTRAINT') {
      return res.status(409).json({ error: 'Backup name already exists' });
    }
    logger.error('Create backup error:', error);
    res.status(500).json({ error: 'Failed to create backup' });
  }
});

router.put('/:id', (req, res) => {
  try {
    const { name, repository_id, source_paths, exclude_patterns, tags, options } = req.body;

    const existingBackup = db.prepare('SELECT * FROM backups WHERE id = ?').get(req.params.id);
    if (!existingBackup) {
      return res.status(404).json({ error: 'Backup not found' });
    }

    db.prepare(`
      UPDATE backups 
      SET name = ?, repository_id = ?, source_paths = ?, exclude_patterns = ?, tags = ?, options = ?
      WHERE id = ?
    `).run(
      name || existingBackup.name,
      repository_id || existingBackup.repository_id,
      source_paths ? JSON.stringify(source_paths) : existingBackup.source_paths,
      exclude_patterns !== undefined ? JSON.stringify(exclude_patterns) : existingBackup.exclude_patterns,
      tags !== undefined ? JSON.stringify(tags) : existingBackup.tags,
      options !== undefined ? JSON.stringify(options) : existingBackup.options,
      req.params.id
    );

    logger.info(`Backup ${req.params.id} updated successfully`);
    res.json({ message: 'Backup updated successfully' });
  } catch (error) {
    logger.error('Update backup error:', error);
    res.status(500).json({ error: 'Failed to update backup' });
  }
});

router.delete('/:id', (req, res) => {
  try {
    const result = db.prepare('DELETE FROM backups WHERE id = ?').run(req.params.id);

    if (result.changes === 0) {
      return res.status(404).json({ error: 'Backup not found' });
    }

    db.prepare('DELETE FROM schedules WHERE backup_id = ?').run(req.params.id);
    db.prepare('DELETE FROM operation_logs WHERE backup_id = ?').run(req.params.id);

    logger.info(`Backup ${req.params.id} deleted successfully`);
    res.json({ message: 'Backup deleted successfully' });
  } catch (error) {
    logger.error('Delete backup error:', error);
    res.status(500).json({ error: 'Failed to delete backup' });
  }
});

router.post('/:id/run', async (req, res) => {
  try {
    const backup = db.prepare('SELECT * FROM backups WHERE id = ?').get(req.params.id);

    if (!backup) {
      return res.status(404).json({ error: 'Backup not found' });
    }

    const { createTask, executeTask } = require('./task');
    
    const taskId = createTask('backup', backup.repository_id, req.params.id);
    logger.info(`Backup ${backup.name} task created`);
    
    executeTask(taskId).catch(err => logger.error('Task execution error:', err));
    
    res.json({ message: 'Backup task started', task_id: taskId });
  } catch (error) {
    logger.error('Run backup error:', error);
    res.status(500).json({ error: 'Failed to run backup' });
  }
});

router.get('/:id/snapshots', async (req, res) => {
  try {
    const backup = db.prepare(`
      SELECT b.*, r.url, r.password, r.env_vars
      FROM backups b
      JOIN repositories r ON b.repository_id = r.id
      WHERE b.id = ?
    `).get(req.params.id);

    if (!backup) {
      return res.status(404).json({ error: 'Backup not found' });
    }

    const restic = new ResticService(
      backup.url,
      backup.password,
      backup.env_vars ? JSON.parse(backup.env_vars) : {}
    );

    const result = await restic.snapshots();
    const snapshots = JSON.parse(result.stdout);

    res.json(snapshots);
  } catch (error) {
    logger.error('Get snapshots error:', error);
    res.status(500).json({ error: 'Failed to get snapshots' });
  }
});

router.get('/:id/logs', (req, res) => {
  try {
    const logs = db.prepare(`
      SELECT * FROM backup_logs 
      WHERE backup_id = ?
      ORDER BY started_at DESC
      LIMIT 100
    `).all(req.params.id);

    res.json(logs);
  } catch (error) {
    logger.error('Get backup logs error:', error);
    res.status(500).json({ error: 'Failed to get backup logs' });
  }
});

module.exports = router;