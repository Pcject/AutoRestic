const express = require('express');
const { db } = require('../config/database');
const authMiddleware = require('../middleware/auth');
const logger = require('../utils/logger');

const router = express.Router();

router.use(authMiddleware);

router.get('/', (req, res) => {
  try {
    const { type, repository_id, backup_id, status, limit = 100, offset = 0, exclude_cached } = req.query;

    let query = 'SELECT * FROM operation_logs';
    const params = [];
    const conditions = [];

    if (type) {
      conditions.push(' type = ?');
      params.push(type);
    }

    if (repository_id) {
      conditions.push(' repository_id = ?');
      params.push(repository_id);
    }

    if (backup_id) {
      conditions.push(' backup_id = ?');
      params.push(backup_id);
    }

    if (status) {
      conditions.push(' status = ?');
      params.push(status);
    }

    if (exclude_cached === 'true') {
      conditions.push(" message NOT LIKE '%(cached)%'");
    }

    if (conditions.length > 0) {
      query += ' WHERE ' + conditions.join(' AND');
    }

    query += ' ORDER BY started_at DESC LIMIT ? OFFSET ?';
    params.push(parseInt(limit), parseInt(offset));

    const logs = db.prepare(query).all(...params);

    res.json(logs);
  } catch (error) {
    logger.error('Get logs error:', error);
    res.status(500).json({ error: 'Failed to get logs' });
  }
});

router.get('/stats', (req, res) => {
  try {
    const stats = db.prepare(`
      SELECT 
        COUNT(*) as total,
        SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
        SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
        SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) as running,
        AVG(CASE WHEN duration IS NOT NULL THEN duration ELSE NULL END) as avg_duration
      FROM operation_logs
    `).get();

    res.json(stats);
  } catch (error) {
    logger.error('Get log stats error:', error);
    res.status(500).json({ error: 'Failed to get log stats' });
  }
});

router.get('/:id', (req, res) => {
  try {
    const log = db.prepare('SELECT * FROM operation_logs WHERE id = ?').get(req.params.id);

    if (!log) {
      return res.status(404).json({ error: 'Log not found' });
    }

    res.json(log);
  } catch (error) {
    logger.error('Get log error:', error);
    res.status(500).json({ error: 'Failed to get log' });
  }
});

module.exports = router;