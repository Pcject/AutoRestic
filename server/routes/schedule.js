const express = require('express');
const { db } = require('../config/database');
const authMiddleware = require('../middleware/auth');
const { addSchedule, removeSchedule, updateSchedule } = require('../services/scheduler');
const logger = require('../utils/logger');

const router = express.Router();

router.use(authMiddleware);

router.get('/', (req, res) => {
  try {
    const schedules = db.prepare(`
      SELECT *
      FROM schedules
      ORDER BY created_at DESC
    `).all();

    const formattedSchedules = schedules.map(schedule => {
      let backupIds = [];
      let backup_name = '';
      let repository_name = '';
      
      try {
        if (schedule.backup_ids) {
          const parsed = JSON.parse(schedule.backup_ids);
          if (Array.isArray(parsed)) {
            backupIds = parsed;
          } else if (typeof parsed === 'number') {
            backupIds = [parsed];
          } else if (typeof parsed === 'string' && !isNaN(parseInt(parsed))) {
            backupIds = [parseInt(parsed)];
          }
        }
      } catch (parseError) {
        logger.warn(`Failed to parse backup_ids for schedule ${schedule.id}:`, parseError);
        if (schedule.backup_id) {
          backupIds = [schedule.backup_id];
        }
      }
      
      let backups = [];
      if (backupIds.length > 0) {
        try {
          backups = db.prepare(`
            SELECT b.*, r.name as repository_name
            FROM backups b
            JOIN repositories r ON b.repository_id = r.id
            WHERE b.id IN (${backupIds.map(() => '?').join(',')})
          `).all(...backupIds);
          backup_name = backups.map(b => b.name).join(', ');
          repository_name = [...new Set(backups.map(b => b.repository_name))].join(', ');
        } catch (dbError) {
          logger.warn(`Failed to get backups for schedule ${schedule.id}:`, dbError);
          backups = [];
        }
      }
      
      return {
        ...schedule,
        backup_ids: backupIds,
        backup_names: backups.map(b => b.name),
        repository_names: backups.map(b => b.repository_name),
        backup_name: backup_name,
        repository_name: repository_name
      };
    });

    res.json(formattedSchedules);
  } catch (error) {
    logger.error('Get schedules error:', error);
    res.status(500).json({ error: 'Failed to get schedules', details: error.message });
  }
});

router.get('/:id', (req, res) => {
  try {
    const schedule = db.prepare('SELECT * FROM schedules WHERE id = ?').get(req.params.id);

    if (!schedule) {
      return res.status(404).json({ error: 'Schedule not found' });
    }

    let backupIds = [];
    try {
      if (schedule.backup_ids) {
        const parsed = JSON.parse(schedule.backup_ids);
        if (Array.isArray(parsed)) {
          backupIds = parsed;
        } else if (typeof parsed === 'number') {
          backupIds = [parsed];
        } else if (typeof parsed === 'string' && !isNaN(parseInt(parsed))) {
          backupIds = [parseInt(parsed)];
        }
      }
    } catch (parseError) {
      logger.warn(`Failed to parse backup_ids for schedule ${schedule.id}:`, parseError);
      if (schedule.backup_id) {
        backupIds = [schedule.backup_id];
      }
    }

    res.json({
      ...schedule,
      backup_ids: backupIds
    });
  } catch (error) {
    logger.error('Get schedule error:', error);
    res.status(500).json({ error: 'Failed to get schedule' });
  }
});

router.post('/', (req, res) => {
  try {
    const { backup_ids, cron_expression, enabled } = req.body;

    if (!backup_ids || !Array.isArray(backup_ids) || backup_ids.length === 0 || !cron_expression) {
      return res.status(400).json({ error: 'backup_ids (array) and cron_expression are required' });
    }

    for (const backupId of backup_ids) {
      const backup = db.prepare('SELECT * FROM backups WHERE id = ?').get(backupId);
      if (!backup) {
        return res.status(404).json({ error: `Backup not found for id ${backupId}` });
      }
    }

    const result = db.prepare(`
      INSERT INTO schedules (backup_ids, cron_expression, enabled)
      VALUES (?, ?, ?)
    `).run(
      JSON.stringify(backup_ids),
      cron_expression,
      enabled !== undefined ? (enabled ? 1 : 0) : 1
    );

    addSchedule(result.lastInsertRowid);
    logger.info(`Schedule created for backups ${backup_ids.join(', ')}`);
    res.status(201).json({ message: 'Schedule created successfully', id: result.lastInsertRowid });
  } catch (error) {
    logger.error('Create schedule error:', error);
    res.status(500).json({ error: 'Failed to create schedule' });
  }
});

router.put('/:id', (req, res) => {
  try {
    const { backup_ids, cron_expression, enabled } = req.body;

    const existingSchedule = db.prepare('SELECT * FROM schedules WHERE id = ?').get(req.params.id);
    if (!existingSchedule) {
      return res.status(404).json({ error: 'Schedule not found' });
    }

    if (backup_ids) {
      if (!Array.isArray(backup_ids) || backup_ids.length === 0) {
        return res.status(400).json({ error: 'backup_ids must be a non-empty array' });
      }
      for (const backupId of backup_ids) {
        const backup = db.prepare('SELECT * FROM backups WHERE id = ?').get(backupId);
        if (!backup) {
          return res.status(404).json({ error: `Backup not found for id ${backupId}` });
        }
      }
    }

    db.prepare(`
      UPDATE schedules 
      SET backup_ids = ?, cron_expression = ?, enabled = ?
      WHERE id = ?
    `).run(
      backup_ids ? JSON.stringify(backup_ids) : existingSchedule.backup_ids,
      cron_expression || existingSchedule.cron_expression,
      enabled !== undefined ? (enabled ? 1 : 0) : existingSchedule.enabled,
      req.params.id
    );

    updateSchedule(req.params.id);
    logger.info(`Schedule ${req.params.id} updated successfully`);
    res.json({ message: 'Schedule updated successfully' });
  } catch (error) {
    logger.error('Update schedule error:', error);
    res.status(500).json({ error: 'Failed to update schedule' });
  }
});

router.delete('/:id', (req, res) => {
  try {
    const result = db.prepare('DELETE FROM schedules WHERE id = ?').run(req.params.id);

    if (result.changes === 0) {
      return res.status(404).json({ error: 'Schedule not found' });
    }

    removeSchedule(req.params.id);
    logger.info(`Schedule ${req.params.id} deleted successfully`);
    res.json({ message: 'Schedule deleted successfully' });
  } catch (error) {
    logger.error('Delete schedule error:', error);
    res.status(500).json({ error: 'Failed to delete schedule' });
  }
});

router.post('/:id/toggle', (req, res) => {
  try {
    const schedule = db.prepare('SELECT * FROM schedules WHERE id = ?').get(req.params.id);

    if (!schedule) {
      return res.status(404).json({ error: 'Schedule not found' });
    }

    const newEnabled = schedule.enabled ? 0 : 1;

    db.prepare('UPDATE schedules SET enabled = ? WHERE id = ?').run(newEnabled, req.params.id);

    if (newEnabled) {
      addSchedule(req.params.id);
    } else {
      removeSchedule(req.params.id);
    }

    logger.info(`Schedule ${req.params.id} toggled to ${newEnabled ? 'enabled' : 'disabled'}`);
    res.json({ message: 'Schedule toggled successfully', enabled: newEnabled === 1 });
  } catch (error) {
    logger.error('Toggle schedule error:', error);
    res.status(500).json({ error: 'Failed to toggle schedule' });
  }
});

module.exports = router;