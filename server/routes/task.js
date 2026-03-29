const express = require('express');
const { db } = require('../config/database');
const authMiddleware = require('../middleware/auth');
const ResticService = require('../services/restic');
const OperationLogService = require('../services/operationLog');
const logger = require('../utils/logger');

const router = express.Router();

router.use(authMiddleware);

const addTaskLog = (taskId, level, message) => {
  db.prepare('INSERT INTO task_logs (task_id, level, message) VALUES (?, ?, ?)').run(
    taskId, level, message
  );
};

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

const updateTaskStatus = (taskId, status, result = null, errorMessage = null) => {
  const updates = { status };
  let query = 'UPDATE tasks SET status = ?';
  const params = [status];
  
  if (result) {
    if (result.stdout !== undefined) {
      query += ', output = ?';
      params.push(result.stdout);
    }
    if (result.stderr !== undefined) {
      query += ', error_message = ?';
      params.push(result.stderr);
    }
  } else if (errorMessage) {
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

const executeTask = async (taskId) => {
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
        const options = backup.options ? JSON.parse(backup.options) : {};
        
        if (backup.exclude_patterns) {
          options.exclude = JSON.parse(backup.exclude_patterns);
        }
        if (backup.tags) {
          options.tags = JSON.parse(backup.tags);
        }
        
        addTaskLog(taskId, 'info', `开始备份: ${backup.name}`);
        result = await OperationLogService.logOperation({
          type: 'backup',
          repository_id: backup.repository_id,
          backup_id: task.backup_id,
          message: `Backup: ${backup.name}`,
          operation: async () => {
            const backupResult = await restic.backup(sourcePaths, options);
            
            const snapshotMatch = backupResult.stdout.match(/snapshot\s+([a-f0-9]+)/);
            if (snapshotMatch) {
              backupResult.snapshot_id = snapshotMatch[1];
            }
            
            return backupResult;
          }
        });
        addTaskLog(taskId, 'info', `备份完成: ${backup.name}`);
        updateTaskStatus(taskId, 'completed', result);
        break;
      }

      case 'restore': {
        const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(task.repository_id);
        
        restic = new ResticService(
          repository.url,
          repository.password,
          repository.env_vars ? JSON.parse(repository.env_vars) : {}
        );
        
        const options = {};
        if (config.include) {
          options.include = Array.isArray(config.include) ? config.include : [config.include];
        }
        
        addTaskLog(taskId, 'info', `开始恢复快照: ${config.snapshot_id}`);
        result = await OperationLogService.logOperation({
          type: 'restore',
          repository_id: task.repository_id,
          snapshot_id: config.snapshot_id,
          message: `Restore snapshot to ${config.target}`,
          operation: async () => restic.restore(config.snapshot_id, config.target, options)
        });
        addTaskLog(taskId, 'info', `恢复完成: ${config.snapshot_id}`);
        updateTaskStatus(taskId, 'completed', result);
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
        result = await OperationLogService.logOperation({
          type: 'check',
          repository_id: task.repository_id,
          message: 'Check repository',
          operation: async () => restic.check(config)
        });
        addTaskLog(taskId, 'info', '仓库检查完成');
        updateTaskStatus(taskId, 'completed', result);
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
        result = await OperationLogService.logOperation({
          type: 'prune',
          repository_id: task.repository_id,
          message: 'Prune repository',
          operation: async () => restic.prune()
        });
        addTaskLog(taskId, 'info', '仓库清理完成');
        updateTaskStatus(taskId, 'completed', result);
        break;
      }

      case 'init': {
        const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(task.repository_id);
        
        restic = new ResticService(
          repository.url,
          repository.password,
          repository.env_vars ? JSON.parse(repository.env_vars) : {}
        );
        
        addTaskLog(taskId, 'info', '开始初始化仓库');
        result = await OperationLogService.logOperation({
          type: 'init',
          repository_id: task.repository_id,
          message: 'Initialize repository',
          operation: async () => restic.init()
        });
        db.prepare('UPDATE repositories SET initialized = 1 WHERE id = ?').run(task.repository_id);
        addTaskLog(taskId, 'info', '仓库初始化完成');
        updateTaskStatus(taskId, 'completed', result);
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
    updateTaskStatus(taskId, 'failed', error);
  }
};

router.get('/', (req, res) => {
  try {
    const { type, status, limit = 50 } = req.query;
    
    let query = 'SELECT * FROM tasks';
    const params = [];
    const conditions = [];
    
    if (type) {
      conditions.push('type = ?');
      params.push(type);
    }
    if (status) {
      conditions.push('status = ?');
      params.push(status);
    }
    
    if (conditions.length > 0) {
      query += ' WHERE ' + conditions.join(' AND ');
    }
    
    query += ' ORDER BY created_at DESC LIMIT ?';
    params.push(limit);
    
    const tasks = db.prepare(query).all(...params);
    
    const formattedTasks = tasks.map(task => ({
      ...task,
      config: task.config ? JSON.parse(task.config) : null
    }));

    res.json(formattedTasks);
  } catch (error) {
    logger.error('Get tasks error:', error);
    res.status(500).json({ error: 'Failed to get tasks' });
  }
});

router.get('/:id', (req, res) => {
  try {
    const task = db.prepare('SELECT * FROM tasks WHERE id = ?').get(req.params.id);

    if (!task) {
      return res.status(404).json({ error: 'Task not found' });
    }

    const formattedTask = {
      ...task,
      config: task.config ? JSON.parse(task.config) : null
    };

    res.json(formattedTask);
  } catch (error) {
    logger.error('Get task error:', error);
    res.status(500).json({ error: 'Failed to get task' });
  }
});

router.get('/:id/logs', (req, res) => {
  try {
    const logs = db.prepare('SELECT * FROM task_logs WHERE task_id = ? ORDER BY created_at ASC').all(req.params.id);
    res.json(logs);
  } catch (error) {
    logger.error('Get task logs error:', error);
    res.status(500).json({ error: 'Failed to get task logs' });
  }
});

router.post('/backup', async (req, res) => {
  try {
    const { backup_id } = req.body;
    
    if (!backup_id) {
      return res.status(400).json({ error: 'backup_id is required' });
    }

    const taskId = createTask('backup', null, backup_id);
    res.status(201).json({ message: 'Backup task created', task_id: taskId });
    
    executeTask(taskId).catch(err => logger.error('Task execution error:', err));
  } catch (error) {
    logger.error('Create backup task error:', error);
    res.status(500).json({ error: 'Failed to create backup task' });
  }
});

router.post('/restore', async (req, res) => {
  try {
    const { repository_id, snapshot_id, target, include } = req.body;
    
    if (!repository_id || !snapshot_id || !target) {
      return res.status(400).json({ error: 'repository_id, snapshot_id, and target are required' });
    }

    const config = { snapshot_id, target };
    if (include) {
      config.include = include;
    }

    const taskId = createTask('restore', repository_id, null, config);
    res.status(201).json({ message: 'Restore task created', task_id: taskId });
    
    executeTask(taskId).catch(err => logger.error('Task execution error:', err));
  } catch (error) {
    logger.error('Create restore task error:', error);
    res.status(500).json({ error: 'Failed to create restore task' });
  }
});

router.post('/check', async (req, res) => {
  try {
    const { repository_id, ...options } = req.body;
    
    if (!repository_id) {
      return res.status(400).json({ error: 'repository_id is required' });
    }

    const taskId = createTask('check', repository_id, null, options);
    res.status(201).json({ message: 'Check task created', task_id: taskId });
    
    executeTask(taskId).catch(err => logger.error('Task execution error:', err));
  } catch (error) {
    logger.error('Create check task error:', error);
    res.status(500).json({ error: 'Failed to create check task' });
  }
});

router.post('/prune', async (req, res) => {
  try {
    const { repository_id } = req.body;
    
    if (!repository_id) {
      return res.status(400).json({ error: 'repository_id is required' });
    }

    const taskId = createTask('prune', repository_id);
    res.status(201).json({ message: 'Prune task created', task_id: taskId });
    
    executeTask(taskId).catch(err => logger.error('Task execution error:', err));
  } catch (error) {
    logger.error('Create prune task error:', error);
    res.status(500).json({ error: 'Failed to create prune task' });
  }
});

router.post('/init', async (req, res) => {
  try {
    const { repository_id } = req.body;
    
    if (!repository_id) {
      return res.status(400).json({ error: 'repository_id is required' });
    }

    const taskId = createTask('init', repository_id);
    res.status(201).json({ message: 'Init task created', task_id: taskId });
    
    executeTask(taskId).catch(err => logger.error('Task execution error:', err));
  } catch (error) {
    logger.error('Create init task error:', error);
    res.status(500).json({ error: 'Failed to create init task' });
  }
});

module.exports = { router, executeTask, createTask };
