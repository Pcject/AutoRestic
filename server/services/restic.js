require('dotenv').config();
const { spawn } = require('child_process');
const logger = require('../utils/logger');

const RESTIC_PATH = process.env.RESTIC_PATH || 'restic';

class ResticService {
  constructor(repository, password, envVars = {}) {
    this.repository = repository;
    this.password = password;
    this.envVars = envVars;
  }

  getEnv() {
    return {
      ...this.envVars,
      RESTIC_REPOSITORY: this.repository,
      RESTIC_PASSWORD: this.password
    };
  }

  async runCommand(args, options = {}) {
    return new Promise((resolve, reject) => {
      const startTime = Date.now();
      let stdout = '';
      let stderr = '';

      const fullCommand = [RESTIC_PATH, ...args].join(' ');

      const childProcess = spawn(RESTIC_PATH, args, {
        env: { ...process.env, ...this.getEnv() },
        ...options
      });

      childProcess.stdout.on('data', (data) => {
        stdout += data.toString();
      });

      childProcess.stderr.on('data', (data) => {
        stderr += data.toString();
      });

      childProcess.on('close', (code) => {
        const duration = Date.now() - startTime;
        
        if (code === 0) {
          resolve({ 
            success: true, 
            stdout: stdout.trim(), 
            stderr: stderr.trim(),
            command: fullCommand,
            duration 
          });
        } else {
          reject({ 
            success: false, 
            code, 
            stdout: stdout.trim(), 
            stderr: stderr.trim(),
            command: fullCommand,
            duration 
          });
        }
      });

      childProcess.on('error', (error) => {
        reject({ 
          success: false, 
          error: error.message,
          command: fullCommand,
          duration: Date.now() - startTime 
        });
      });
    });
  }

  async init() {
    logger.info(`Initializing repository: ${this.repository}`);
    return this.runCommand(['init']);
  }

  async backup(paths, options = {}) {
    const args = ['backup', ...paths];
    
    if (options.exclude) {
      options.exclude.forEach(pattern => {
        args.push('--exclude', pattern);
      });
    }

    if (options.excludeFile) {
      args.push('--exclude-file', options.excludeFile);
    }

    if (options.tags) {
      options.tags.forEach(tag => {
        args.push('--tag', tag);
      });
    }

    if (options.host) {
      args.push('--host', options.host);
    }

    if (options.dryRun) {
      args.push('--dry-run');
    }

    if (options.excludeCaches) {
      args.push('--exclude-caches');
    }

    if (options.excludeLargerThan) {
      args.push('--exclude-larger-than', options.excludeLargerThan);
    }

    if (options.filesFrom) {
      args.push('--files-from', options.filesFrom);
    }

    if (options.force) {
      args.push('--force');
    }

    if (options.iexclude) {
      options.iexclude.forEach(pattern => {
        args.push('--iexclude', pattern);
      });
    }

    if (options.iexcludeFile) {
      args.push('--iexclude-file', options.iexcludeFile);
    }

    if (options.ignoreCtime) {
      args.push('--ignore-ctime');
    }

    if (options.ignoreInode) {
      args.push('--ignore-inode');
    }

    if (options.noScan) {
      args.push('--no-scan');
    }

    if (options.oneFileSystem) {
      args.push('--one-file-system');
    }

    if (options.parent) {
      args.push('--parent', options.parent);
    }

    if (options.readConcurrency) {
      args.push('--read-concurrency', options.readConcurrency.toString());
    }

    if (options.skipIfUnchanged) {
      args.push('--skip-if-unchanged');
    }

    if (options.stdin) {
      args.push('--stdin');
    }

    if (options.stdinFilename) {
      args.push('--stdin-filename', options.stdinFilename);
    }

    if (options.time) {
      args.push('--time', options.time);
    }

    if (options.withAtime) {
      args.push('--with-atime');
    }

    if (options.compression) {
      args.push('--compression', options.compression);
    }

    if (options.limitDownload) {
      args.push('--limit-download', options.limitDownload.toString());
    }

    if (options.limitUpload) {
      args.push('--limit-upload', options.limitUpload.toString());
    }

    if (options.noCache) {
      args.push('--no-cache');
    }

    if (options.noExtraVerify) {
      args.push('--no-extra-verify');
    }

    if (options.noLock) {
      args.push('--no-lock');
    }

    if (options.packSize) {
      args.push('--pack-size', options.packSize.toString());
    }

    if (options.verbose) {
      args.push(`--verbose=${options.verbose}`);
    }

    logger.info(`Starting backup for paths: ${paths.join(', ')}`);
    return this.runCommand(args);
  }

  async restore(snapshotId, target, options = {}) {
    const args = ['restore', snapshotId, '--target', target];

    if (options.include) {
      options.include.forEach(pattern => {
        args.push('--include', pattern);
      });
    }

    if (options.exclude) {
      options.exclude.forEach(pattern => {
        args.push('--exclude', pattern);
      });
    }

    logger.info(`Restoring snapshot ${snapshotId} to ${target}`);
    return this.runCommand(args);
  }

  async snapshots(options = {}) {
    const args = ['snapshots', '--json'];

    if (options.last) {
      args.push('--last', options.last.toString());
    }

    if (options.filter) {
      Object.entries(options.filter).forEach(([key, value]) => {
        args.push(`--${key}`, value);
      });
    }

    return this.runCommand(args);
  }

  async forget(options = {}) {
    const args = ['forget'];

    if (options.keepLast) {
      args.push('--keep-last', options.keepLast.toString());
    }

    if (options.keepHourly) {
      args.push('--keep-hourly', options.keepHourly.toString());
    }

    if (options.keepDaily) {
      args.push('--keep-daily', options.keepDaily.toString());
    }

    if (options.keepWeekly) {
      args.push('--keep-weekly', options.keepWeekly.toString());
    }

    if (options.keepMonthly) {
      args.push('--keep-monthly', options.keepMonthly.toString());
    }

    if (options.keepYearly) {
      args.push('--keep-yearly', options.keepYearly.toString());
    }

    if (options.keepWithin) {
      args.push('--keep-within', options.keepWithin);
    }

    if (options.prune) {
      args.push('--prune');
    }

    if (options.dryRun) {
      args.push('--dry-run');
    }

    logger.info('Running forget command');
    return this.runCommand(args);
  }

  async check(options = {}) {
    const args = ['check'];

    if (options.readData) {
      args.push('--read-data');
    }

    if (options.readDataSubset) {
      args.push('--read-data-subset', options.readDataSubset);
    }

    logger.info('Running repository check');
    return this.runCommand(args);
  }

  async prune() {
    logger.info('Running prune');
    return this.runCommand(['prune']);
  }

  async stats(options = {}) {
    const args = ['stats', '--json'];

    if (options.snapshotId) {
      args.push(options.snapshotId);
    }

    if (options.mode) {
      args.push('--mode', options.mode);
    }

    return this.runCommand(args);
  }

  async list(snapshotId, path = '/', json = true) {
    const args = ['ls', snapshotId, path];
    if (json) {
      args.push('--json');
    }
    return this.runCommand(args);
  }

  async find(pattern, options = {}) {
    const args = ['find', pattern];

    if (options.snapshot) {
      args.push('--snapshot', options.snapshot);
    }

    if (options.oldest) {
      args.push('--oldest', options.oldest);
    }

    if (options.newest) {
      args.push('--newest', options.newest);
    }

    return this.runCommand(args);
  }

  async mount(snapshotId, mountPoint) {
    logger.info(`Mounting ${snapshotId} to ${mountPoint}`);
    return this.runCommand(['mount', mountPoint], { detached: true });
  }

  async cat(snapshotId, path) {
    return this.runCommand(['cat', snapshotId, path]);
  }

  async diff(snapshotId1, snapshotId2) {
    return this.runCommand(['diff', snapshotId1, snapshotId2]);
  }

  async dump(snapshotId, path, format = 'tar') {
    return this.runCommand(['dump', '--format', format, snapshotId, path]);
  }

  async generate() {
    return this.runCommand(['generate', '--json']);
  }

  async cache() {
    return this.runCommand(['cache', '--cleanup']);
  }

  async version() {
    return this.runCommand(['version']);
  }
}

module.exports = ResticService;