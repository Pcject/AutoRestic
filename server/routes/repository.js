const express = require('express');
const { db } = require('../config/database');
const authMiddleware = require('../middleware/auth');
const ResticService = require('../services/restic');
const OperationLogService = require('../services/operationLog');
const logger = require('../utils/logger');

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

const executeInitTask = async (taskId) => {
  const task = db.prepare('SELECT * FROM tasks WHERE id = ?').get(taskId);
  if (!task) return;

  updateTaskStatus(taskId, 'running');

  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(task.repository_id);
    
    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const result = await OperationLogService.logOperation({
      type: 'init',
      repository_id: task.repository_id,
      message: 'Initialize repository',
      operation: async () => restic.init()
    });
    
    db.prepare('UPDATE repositories SET initialized = 1 WHERE id = ?').run(task.repository_id);
    updateTaskStatus(taskId, 'completed', result);
  } catch (error) {
    logger.error('Init task execution error:', error);
    const errorMsg = error.stderr || error.message || 'Unknown error';
    updateTaskStatus(taskId, 'failed', null, errorMsg);
  }
};

const router = express.Router();

router.use(authMiddleware);

const snapshotsCache = new Map();

router.get('/', (req, res) => {
  try {
    const repositories = db.prepare('SELECT * FROM repositories ORDER BY created_at DESC').all();

    const formattedRepositories = repositories.map(repo => ({
      ...repo,
      env_vars: repo.env_vars ? JSON.parse(repo.env_vars) : {}
    }));

    res.json(formattedRepositories);
  } catch (error) {
    logger.error('Get repositories error:', error);
    res.status(500).json({ error: 'Failed to get repositories' });
  }
});

router.get('/:id', (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const formattedRepository = {
      ...repository,
      env_vars: repository.env_vars ? JSON.parse(repository.env_vars) : {}
    };

    res.json(formattedRepository);
  } catch (error) {
    logger.error('Get repository error:', error);
    res.status(500).json({ error: 'Failed to get repository' });
  }
});

router.post('/', async (req, res) => {
  try {
    const { name, type, url, password, env_vars, auto_init } = req.body;

    if (!name || !type || !url || !password) {
      return res.status(400).json({ error: 'Name, type, url, and password are required' });
    }

    let isInitialized = 0;

    if (auto_init) {
      try {
        const restic = new ResticService(
          url,
          password,
          env_vars ? JSON.parse(env_vars) : {}
        );

        await restic.snapshots();
        isInitialized = 1;
        logger.info(`Repository ${name} already exists, skipping initialization`);
      } catch (checkError) {
        logger.info(`Repository ${name} does not exist, will initialize`);
      }
    }

    const result = db.prepare(`
      INSERT INTO repositories (name, type, url, password, env_vars, initialized)
      VALUES (?, ?, ?, ?, ?, ?)
    `).run(
      name,
      type,
      url,
      password,
      env_vars ? JSON.stringify(env_vars) : null,
      isInitialized
    );

    const repoId = result.lastInsertRowid;

    if (auto_init && !isInitialized) {
      const taskId = createTask('init', repoId);
      logger.info(`Repository ${name} created and init task scheduled`);
      executeInitTask(taskId).catch(err => logger.error('Init task execution error:', err));
      res.status(201).json({ message: 'Repository created successfully, init task started', id: repoId, task_id: taskId });
    } else if (auto_init && isInitialized) {
      logger.info(`Repository ${name} created successfully, existing repository detected`);
      res.status(201).json({ message: 'Repository created successfully, existing repository detected', id: repoId });
    } else {
      logger.info(`Repository ${name} created successfully`);
      res.status(201).json({ message: 'Repository created successfully', id: repoId });
    }
  } catch (error) {
    if (error.code === 'SQLITE_CONSTRAINT') {
      return res.status(409).json({ error: 'Repository name already exists' });
    }
    logger.error('Create repository error:', error);
    res.status(500).json({ error: 'Failed to create repository' });
  }
});

router.put('/:id', (req, res) => {
  try {
    const { name, type, url, password, env_vars } = req.body;

    const existingRepo = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);
    if (!existingRepo) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    db.prepare(`
      UPDATE repositories 
      SET name = ?, type = ?, url = ?, password = ?, env_vars = ?
      WHERE id = ?
    `).run(
      name || existingRepo.name,
      type || existingRepo.type,
      url || existingRepo.url,
      password || existingRepo.password,
      env_vars !== undefined ? JSON.stringify(env_vars) : existingRepo.env_vars,
      req.params.id
    );

    logger.info(`Repository ${req.params.id} updated successfully`);
    res.json({ message: 'Repository updated successfully' });
  } catch (error) {
    logger.error('Update repository error:', error);
    res.status(500).json({ error: 'Failed to update repository' });
  }
});

router.delete('/:id', (req, res) => {
  try {
    const result = db.prepare('DELETE FROM repositories WHERE id = ?').run(req.params.id);

    if (result.changes === 0) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    logger.info(`Repository ${req.params.id} deleted successfully`);
    res.json({ message: 'Repository deleted successfully' });
  } catch (error) {
    logger.error('Delete repository error:', error);
    res.status(500).json({ error: 'Failed to delete repository' });
  }
});

router.post('/:id/check-init', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    try {
      await OperationLogService.logOperation({
        type: 'snapshots',
        repository_id: req.params.id,
        message: 'Check repository init status',
        operation: async () => restic.snapshots()
      });
      db.prepare('UPDATE repositories SET initialized = 1 WHERE id = ?').run(req.params.id);
      res.json({ initialized: true });
    } catch (error) {
      db.prepare('UPDATE repositories SET initialized = 0 WHERE id = ?').run(req.params.id);
      res.json({ initialized: false });
    }
  } catch (error) {
    logger.error('Check repository init status error:', error);
    res.status(500).json({ error: 'Failed to check repository status' });
  }
});

router.post('/:id/init', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const taskId = createTask('init', req.params.id);
    logger.info(`Repository ${repository.name} init task scheduled`);
    executeInitTask(taskId).catch(err => logger.error('Init task execution error:', err));
    res.json({ message: 'Init task started', task_id: taskId });
  } catch (error) {
    logger.error('Init repository error:', error);
    res.status(500).json({ error: 'Failed to initialize repository', details: error.stderr || error.error });
  }
});

const clearSnapshotsCache = (repoId) => {
  snapshotsCache.delete(repoId);
  logger.info(`Cleared snapshots cache for repo ${repoId}`);
};

router.get('/:id/snapshots', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const snapshotsResult = await OperationLogService.logOperation({
      type: 'snapshots',
      repository_id: req.params.id,
      message: 'List snapshots',
      operation: async () => restic.snapshots()
    });

    let snapshots = JSON.parse(snapshotsResult.stdout);
    logger.info(`Found ${snapshots.length} snapshots`);

    for (const snapshot of snapshots) {
      try {
        logger.info(`Getting stats for snapshot ${snapshot.id}`);
        const statsResult = await OperationLogService.logOperation({
          type: 'stats',
          repository_id: req.params.id,
          snapshot_id: snapshot.id,
          message: `Get stats for snapshot ${snapshot.id}`,
          operation: async () => restic.stats({ snapshotId: snapshot.id, mode: 'restore-size' })
        });
        const stats = JSON.parse(statsResult.stdout);
        snapshot.size = stats.total_size;
        logger.info(`Snapshot ${snapshot.id} size: ${stats.total_size}`);
      } catch (statsError) {
        logger.warn(`Failed to get stats for snapshot ${snapshot.id}:`, statsError);
        snapshot.size = 0;
      }
    }

    res.json(snapshots);
  } catch (error) {
    logger.error('Get repository snapshots error:', error);
    res.status(500).json({ error: 'Failed to get snapshots', details: error.stderr || error.error });
  }
});

router.post('/:id/restore', async (req, res) => {
  try {
    const { snapshot_id, target, include } = req.body;
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    if (!snapshot_id || !target) {
      return res.status(400).json({ error: 'snapshot_id and target are required' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const options = {};
    if (include) {
      options.include = Array.isArray(include) ? include : [include];
    }

    await OperationLogService.logOperation({
      type: 'restore',
      repository_id: req.params.id,
      snapshot_id: snapshot_id,
      message: `Restore snapshot to ${target}`,
      operation: async () => restic.restore(snapshot_id, target, options)
    });

    logger.info(`Snapshot ${snapshot_id} restored to ${target} ${include ? `(include: ${include})` : ''}`);
    res.json({ message: 'Snapshot restored successfully' });
  } catch (error) {
    logger.error('Restore snapshot error:', error);
    res.status(500).json({ error: 'Failed to restore snapshot', details: error.stderr || error.error });
  }
});

router.post('/:id/check', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const result = await OperationLogService.logOperation({
      type: 'check',
      repository_id: req.params.id,
      message: 'Check repository',
      operation: async () => restic.check(req.body)
    });

    logger.info(`Repository ${repository.name} check completed`);
    res.json({ message: 'Repository check completed', result });
  } catch (error) {
    logger.error('Check repository error:', error);
    res.status(500).json({ error: 'Failed to check repository', details: error.stderr || error.error });
  }
});

router.post('/:id/prune', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const result = await OperationLogService.logOperation({
      type: 'prune',
      repository_id: req.params.id,
      message: 'Prune repository',
      operation: async () => restic.prune()
    });

    logger.info(`Repository ${repository.name} prune completed`);
    res.json({ message: 'Repository prune completed', result });
  } catch (error) {
    logger.error('Prune repository error:', error);
    res.status(500).json({ error: 'Failed to prune repository', details: error.stderr || error.error });
  }
});

router.get('/:id/stats', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const [snapshotsResult, rawDataResult, restoreSizeResult] = await Promise.allSettled([
      OperationLogService.logOperation({
        type: 'snapshots',
        repository_id: req.params.id,
        message: 'Get snapshots for stats',
        operation: async () => restic.snapshots()
      }),
      OperationLogService.logOperation({
        type: 'stats',
        repository_id: req.params.id,
        message: 'Get raw data stats',
        operation: async () => restic.stats({ mode: 'raw-data' })
      }),
      OperationLogService.logOperation({
        type: 'stats',
        repository_id: req.params.id,
        message: 'Get restore size stats',
        operation: async () => restic.stats({ mode: 'restore-size' })
      })
    ]);

    const stats = {};

    if (snapshotsResult.status === 'fulfilled') {
      try {
        const snapshots = JSON.parse(snapshotsResult.value.stdout);
        stats.total_snapshots = snapshots.length;
        logger.info(`Found ${snapshots.length} snapshots`);
      } catch (parseError) {
        logger.error('Failed to parse snapshots:', parseError);
        logger.error('Snapshots stdout:', snapshotsResult.value.stdout);
        stats.total_snapshots = 0;
      }
    } else {
      logger.error('Snapshots command failed:', snapshotsResult.reason);
      stats.total_snapshots = 0;
    }

    if (rawDataResult.status === 'fulfilled') {
      try {
        const rawStats = JSON.parse(rawDataResult.value.stdout);
        stats.total_size_compressed = rawStats.total_size;
      } catch (parseError) {
        logger.error('Failed to parse raw stats:', parseError);
        stats.total_size_compressed = 0;
      }
    } else {
      stats.total_size_compressed = 0;
    }

    if (restoreSizeResult.status === 'fulfilled') {
      try {
        const restoreStats = JSON.parse(restoreSizeResult.value.stdout);
        stats.total_size = restoreStats.total_size;
      } catch (parseError) {
        logger.error('Failed to parse restore stats:', parseError);
        stats.total_size = 0;
      }
    } else {
      stats.total_size = 0;
    }

    logger.info('Stats:', stats);
    res.json(stats);
  } catch (error) {
    logger.error('Get repository stats error:', error);
    res.status(500).json({ error: 'Failed to get repository stats', details: error.stderr || error.error });
  }
});

router.post('/:id/forget', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const result = await OperationLogService.logOperation({
      type: 'forget',
      repository_id: req.params.id,
      message: 'Forget snapshots',
      operation: async () => restic.forget(req.body)
    });

    logger.info(`Repository ${repository.name} forget completed`);
    res.json({ message: 'Forget operation completed', result });
  } catch (error) {
    logger.error('Forget error:', error);
    res.status(500).json({ error: 'Failed to forget', details: error.stderr || error.error });
  }
});

router.post('/:id/unlock', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    await OperationLogService.logOperation({
      type: 'unlock',
      repository_id: req.params.id,
      message: 'Unlock repository',
      operation: async () => restic.runCommand(['unlock', '--remove-all'])
    });

    logger.info(`Repository ${repository.name} unlocked successfully`);
    res.json({ message: 'Repository unlocked successfully' });
  } catch (error) {
    logger.error('Unlock repository error:', error);
    res.status(500).json({ error: 'Failed to unlock repository', details: error.stderr || error.error });
  }
});

router.get('/:id/ls/:snapshotId', async (req, res) => {
  try {
    const repository = db.prepare('SELECT * FROM repositories WHERE id = ?').get(req.params.id);

    if (!repository) {
      return res.status(404).json({ error: 'Repository not found' });
    }

    const { path = '/' } = req.query;
    logger.info(`Listing files for snapshot ${req.params.snapshotId}, path: ${path}`);

    const restic = new ResticService(
      repository.url,
      repository.password,
      repository.env_vars ? JSON.parse(repository.env_vars) : {}
    );

    const result = await OperationLogService.logOperation({
      type: 'ls',
      repository_id: req.params.id,
      snapshot_id: req.params.snapshotId,
      message: `List files at ${path}`,
      operation: async () => {
        const listResult = await restic.list(req.params.snapshotId, path);
        logger.info('Restic ls stdout length:', listResult.stdout.length);
        logger.info('Restic ls stdout first 500 chars:', listResult.stdout.substring(0, 500));
        
        const allLines = listResult.stdout
          .split('\n')
          .filter(line => line.trim());
        
        logger.info('Total lines:', allLines.length);
        
        const files = allLines
          .map(line => {
            try {
              return JSON.parse(line);
            } catch (e) {
              logger.warn('Failed to parse line:', line.substring(0, 100));
              return null;
            }
          })
          .filter(item => item !== null);
        
        logger.info('Parsed items:', files.length);
        logger.info('Item types:', files.map(f => f.message_type || f.type));
        
        const nodes = files.filter(item => item.message_type === 'node');
        logger.info('Node items:', nodes.length);
        
        const formattedFiles = nodes
          .filter(file => file.name !== '.')
          .map(file => ({
            name: file.name,
            type: file.type,
            path: file.path,
            size: file.size,
            mtime: file.mtime
          }));

        logger.info('Returning formatted files:', formattedFiles.length);
        return { ...listResult, formattedFiles };
      }
    });

    res.json(result.formattedFiles);
  } catch (error) {
    logger.error('List snapshot files error:', error);
    res.status(500).json({ error: 'Failed to list files', details: error.stderr || error.error });
  }
});

module.exports = { router, clearSnapshotsCache };
