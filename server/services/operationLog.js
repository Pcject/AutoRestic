const { db } = require('../config/database');
const logger = require('../utils/logger');

const formatJsonOutput = (content) => {
  if (!content) return content;
  
  try {
    const lines = content.split('\n');
    const formattedLines = lines.map(line => {
      const trimmedLine = line.trim();
      if (!trimmedLine) return line;
      
      try {
        const parsed = JSON.parse(trimmedLine);
        return JSON.stringify(parsed, null, 2);
      } catch {
        return line;
      }
    });
    return formattedLines.join('\n');
  } catch {
    return content;
  }
};

class OperationLogService {
  static async logOperation({ type, repository_id, backup_id, snapshot_id, message, operation }) {
    logger.info(`===== OperationLogService.logOperation START =====`);
    logger.info(`Type: ${type}, Message: ${message}`);
    logger.info(`Repository ID: ${repository_id}, Backup ID: ${backup_id}, Snapshot ID: ${snapshot_id}`);
    
    const startTime = Date.now();
    
    try {
      logger.info(`Calling operation()...`);
      const result = await operation();
      const duration = Date.now() - startTime;
      
      logger.info(`Operation completed successfully`);
      logger.info(`  - Command: ${result.command}`);
      logger.info(`  - Duration: ${duration}ms`);
      logger.info(`  - Stdout length: ${result.stdout ? result.stdout.length : 0}`);
      logger.info(`  - Stderr length: ${result.stderr ? result.stderr.length : 0}`);
      
      try {
        logger.info(`Preparing to insert into operation_logs...`);
        
        const stmt = db.prepare(`
          INSERT INTO operation_logs 
          (type, repository_id, backup_id, snapshot_id, status, message, duration, restic_command, restic_stdout, restic_stderr, restic_stdout_formatted, restic_stderr_formatted, started_at, completed_at)
          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', '-' || ? || ' milliseconds'), datetime('now'))
        `);
        
        const finalSnapshotId = result.snapshot_id || snapshot_id || null;
        const stdoutFormatted = formatJsonOutput(result.stdout);
        const stderrFormatted = formatJsonOutput(result.stderr);
        const params = [
          type,
          repository_id || null,
          backup_id || null,
          finalSnapshotId,
          'success',
          message,
          duration,
          result.command,
          result.stdout,
          result.stderr,
          stdoutFormatted,
          stderrFormatted,
          duration
        ];
        
        logger.info(`Insert parameters:`, {
          type: params[0],
          repository_id: params[1],
          backup_id: params[2],
          snapshot_id: params[3],
          status: params[4],
          message: params[5],
          duration: params[6],
          restic_command: params[7] ? params[7].substring(0, 100) : null
        });
        
        const insertResult = stmt.run(...params);
        
        logger.info(`SUCCESS: Logged operation to database, insert ID: ${insertResult.lastInsertRowid}`);
        logger.info(`  - Changes: ${insertResult.changes}`);
        
        const verifyStmt = db.prepare('SELECT * FROM operation_logs WHERE id = ?');
        const insertedLog = verifyStmt.get(insertResult.lastInsertRowid);
        logger.info(`Verified inserted log:`, {
          id: insertedLog.id,
          type: insertedLog.type,
          status: insertedLog.status,
          restic_command: insertedLog.restic_command ? insertedLog.restic_command.substring(0, 100) : null
        });
        
      } catch (dbError) {
        logger.error(`FAILED to log operation to database:`, dbError);
        logger.error(`Error message:`, dbError.message);
        logger.error(`Error stack:`, dbError.stack);
      }
      
      logger.info(`===== OperationLogService.logOperation END (SUCCESS) =====`);
      return result;
    } catch (error) {
      const duration = Date.now() - startTime;
      
      logger.error(`Operation failed: ${type}`);
      logger.error(`  - Error: ${error.message || error}`);
      logger.error(`  - Command: ${error.command}`);
      
      try {
        logger.info(`Preparing to insert failed operation into operation_logs...`);
        
        const stmt = db.prepare(`
          INSERT INTO operation_logs 
          (type, repository_id, backup_id, snapshot_id, status, message, duration, restic_command, restic_stdout, restic_stderr, restic_stdout_formatted, restic_stderr_formatted, started_at, completed_at)
          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', '-' || ? || ' milliseconds'), datetime('now'))
        `);
        
        const stdoutFormatted = formatJsonOutput(error.stdout);
        const stderrFormatted = formatJsonOutput(error.stderr);
        const params = [
          type,
          repository_id || null,
          backup_id || null,
          snapshot_id || null,
          'failed',
          message,
          duration,
          error.command,
          error.stdout,
          error.stderr,
          stdoutFormatted,
          stderrFormatted,
          duration
        ];
        
        const insertResult = stmt.run(...params);
        logger.info(`SUCCESS: Logged failed operation to database, insert ID: ${insertResult.lastInsertRowid}`);
        
      } catch (dbError) {
        logger.error(`FAILED to log failed operation to database:`, dbError);
      }
      
      logger.info(`===== OperationLogService.logOperation END (FAILED) =====`);
      throw error;
    }
  }
}

module.exports = OperationLogService;
