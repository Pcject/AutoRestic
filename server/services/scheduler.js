const cron = require('node-cron');
const { db } = require('../config/database');
const logger = require('../utils/logger');

const scheduledTasks = new Map();

function parseBackupIds(schedule) {
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
  return backupIds;
}

const createTask = (type, repositoryId, backupId, config = {}) => {
  const result = db.prepare(`
    INSERT INTO tasks (type, status, repository_id, backup_id, config)
    VALUES (?, 'pending', ?, ?, ?)
  `).run(
    type,
    repositoryId || null,
    backupId || null,
    JSON.stringify(config)
  );
  return result.lastInsertRowid;
};

const updateTaskStatus = (taskId, status, output = null, errorMessage = null) => {
  const updates = { status };
  if (output) updates.output = output;
  if (errorMessage) updates.error_message = errorMessage;
  
  let query = 'UPDATE tasks SET status = ?';
  const params = [status];
  
  if (output) {
    query += ', output = ?';
    params.push(output);
  }
  if (errorMessage) {
    query += ', error_message = ?';
    params.push(errorMessage);
  }
  
  if (status === 'completed' || status === 'failed') {
    query += ', completed_at = CURRENT_TIMESTAMP';
  }
  
  query += ' WHERE id = ?';
  params.push(taskId);
  
  db.prepare(query).run(...params);
};

const addTaskLog = (taskId, level, message) => {
  db.prepare('INSERT INTO task_logs (task_id, level, message) VALUES (?, ?, ?)').run(
    taskId, level, message
  );
};

const executeTask = async (taskId) => {
  const ResticService = require('./restic');
  
  const task = db.prepare('SELECT * FROM tasks WHERE id = ?').get(taskId);
  if (!task) return;

  updateTaskStatus(taskId, 'running');
  addTaskLog(taskId, 'info', `开始执行任务: ${task.type}`);

  try {
    const config = task.config ? JSON.parse(task.config) : {};
    let restic;
    let result;

    switch (task.type) {
      case 'backup': {
        const backup = db.prepare('SELECT * FROM backups WHERE id = ?').get(task.backup_id);
        const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(backup.repository_id);
        
        restic = new ResticService(
          repository.url,
          repository.password,
          repository.env_vars ? JSON.parse(repository.env_vars) : {}
        );
        
        const sourcePaths = JSON.parse(backup.source_paths);
        const options = {};
        
        if (backup.exclude_patterns) {
          options.exclude = JSON.parse(backup.exclude_patterns);
        }
        if (backup.tags) {
          options.tags = JSON.parse(backup.tags);
        }
        
        addTaskLog(taskId, 'info', `开始备份: ${backup.name}`);
        result = await restic.backup(sourcePaths, options);
        addTaskLog(taskId, 'info', `备份完成: ${backup.name}`);
        updateTaskStatus(taskId, 'completed', result.stdout);
        break;
      }

      case 'restore': {
        const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(task.repository_id);
        
        restic = new ResticService(
          repository.url,
          repository.password,
          repository.env_vars ? JSON.parse(repository.env_vars) : {}
        );
        
        addTaskLog(taskId, 'info', `开始恢复快照: ${config.snapshot_id}`);
        result = await restic.restore(config.snapshot_id, config.target);
        addTaskLog(taskId, 'info', `恢复完成: ${config.snapshot_id}`);
        updateTaskStatus(taskId, 'completed', result.stdout);
        break;
      }

      case 'check': {
        const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(task.repository_id);
        
        restic = new ResticService(
          repository.url,
          repository.password,
          repository.env_vars ? JSON.parse(repository.env_vars) : {}
        );
        
        addTaskLog(taskId, 'info', '开始检查仓库');
        result = await restic.check(config);
        addTaskLog(taskId, 'info', '仓库检查完成');
        updateTaskStatus(taskId, 'completed', result.stdout);
        break;
      }

      case 'prune': {
        const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(task.repository_id);
        
        restic = new ResticService(
          repository.url,
          repository.password,
          repository.env_vars ? JSON.parse(repository.env_vars) : {}
        );
        
        addTaskLog(taskId, 'info', '开始清理仓库');
        result = await restic.prune();
        addTaskLog(taskId, 'info', '仓库清理完成');
        updateTaskStatus(taskId, 'completed', result.stdout);
        break;
      }

      default:
        throw new Error(`Unknown task type: ${task.type}`);
    }

    addTaskLog(taskId, 'info', '任务执行成功');
  } catch (error) {
    logger.error('Task execution error:', error);
    const errorMsg = error.stderr || error.message || 'Unknown error';
    addTaskLog(taskId, 'error', `任务执行失败: ${errorMsg}`);
    updateTaskStatus(taskId, 'failed', null, errorMsg);
  }
};

function initScheduler() {
  logger.info('Initializing scheduler...');

  try {
    const schedules = db.prepare(`
      SELECT *
      FROM schedules
      WHERE enabled = 1
    `).all();

    schedules.forEach(schedule => {
      const backupIds = parseBackupIds(schedule);
      const firstBackup = backupIds.length > 0 ? db.prepare('SELECT name FROM backups WHERE id = ?').get(backupIds[0]) : null;
      scheduleTask({
        ...schedule,
        backup_ids: backupIds,
        backup_name: firstBackup?.name || 'Multiple'
      });
    });

    logger.info(`Scheduler initialized with ${scheduledTasks.size} active tasks`);
  } catch (error) {
    logger.error('Failed to initialize scheduler:', error);
  }
}

function scheduleTask(schedule) {
  try {
    if (scheduledTasks.has(schedule.id)) {
      const existingTask = scheduledTasks.get(schedule.id);
      existingTask.stop();
      scheduledTasks.delete(schedule.id);
    }

    const task = cron.schedule(schedule.cron_expression, async () => {
      await executeScheduledBackup(schedule);
    }, {
      scheduled: true,
      timezone: 'Asia/Shanghai'
    });

    scheduledTasks.set(schedule.id, task);
    const backupIdsLog = Array.isArray(schedule.backup_ids) ? schedule.backup_ids.join(', ') : 'unknown';
    logger.info(`Scheduled task ${schedule.id} for backups ${backupIdsLog} with cron: ${schedule.cron_expression}`);
  } catch (error) {
    logger.error(`Failed to schedule task ${schedule.id}:`, error);
  }
}

async function executeScheduledBackup(schedule) {
  let backupIds = [];
  if (Array.isArray(schedule.backup_ids)) {
    backupIds = schedule.backup_ids;
  } else {
    backupIds = parseBackupIds(schedule);
  }
  
  logger.info(`Executing scheduled backups: ${backupIds.join(', ')}`);

  try {
    for (const backupId of backupIds) {
      try {
        const backup = db.prepare('SELECT name FROM backups WHERE id = ?').get(backupId);
        logger.info(`Executing scheduled backup ${backupId} (${backup?.name || ''})`);
        
        const taskId = createTask('backup', null, backupId);
        
        logger.info(`Scheduled backup task created: ${taskId}`);
        
        await executeTask(taskId);
        
        logger.info(`Scheduled backup ${backupId} completed`);
      } catch (error) {
        logger.error(`Failed to execute scheduled backup ${backupId}:`, error);
      }
    }
    
    db.prepare(`
      UPDATE schedules 
      SET last_run = datetime('now'), next_run = datetime('now', '+1 minute')
      WHERE id = ?
    `).run(schedule.id);
    
    logger.info(`All scheduled backups completed`);
  } catch (error) {
    logger.error(`Failed to execute scheduled backups:`, error);
  }
}

function addSchedule(scheduleId) {
  try {
    const schedule = db.prepare(`
      SELECT *
      FROM schedules
      WHERE id = ?
    `).get(scheduleId);

    if (schedule && schedule.enabled) {
      const backupIds = parseBackupIds(schedule);
      const firstBackup = backupIds.length > 0 ? db.prepare('SELECT name FROM backups WHERE id = ?').get(backupIds[0]) : null;
      scheduleTask({
        ...schedule,
        backup_ids: backupIds,
        backup_name: firstBackup?.name || 'Multiple'
      });
    }
  } catch (error) {
    logger.error(`Failed to add schedule ${scheduleId}:`, error);
  }
}

function removeSchedule(scheduleId) {
  try {
    if (scheduledTasks.has(scheduleId)) {
      const task = scheduledTasks.get(scheduleId);
      task.stop();
      scheduledTasks.delete(scheduleId);
      logger.info(`Removed schedule ${scheduleId}`);
    }
  } catch (error) {
    logger.error(`Failed to remove schedule ${scheduleId}:`, error);
  }
}

function updateSchedule(scheduleId) {
  removeSchedule(scheduleId);
  addSchedule(scheduleId);
}

function getScheduledTasks() {
  return Array.from(scheduledTasks.keys());
}

module.exports = {
  initScheduler,
  addSchedule,
  removeSchedule,
  updateSchedule,
  getScheduledTasks
};
