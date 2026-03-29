require('dotenv').config();
const express = require('express');
const cors = require('cors');
const path = require('path');
const { initDatabase } = require('./config/database');
const { initScheduler } = require('./services/scheduler');
const logger = require('./utils/logger');
const authRoutes = require('./routes/auth');
const backupRoutes = require('./routes/backup');
const { router: repositoryRoutes, clearSnapshotsCache } = require('./routes/repository');
const scheduleRoutes = require('./routes/schedule');
const logRoutes = require('./routes/log');
const { router: taskRoutes } = require('./routes/task');

const app = express();
const PORT = process.env.PORT || 3001;

app.use(cors());
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

app.use('/api/auth', authRoutes);
app.use('/api/backups', backupRoutes);
app.use('/api/repositories', repositoryRoutes);
app.use('/api/schedules', scheduleRoutes);
app.use('/api/logs', logRoutes);
app.use('/api/tasks', taskRoutes);

// 始终服务前端文件
const clientDistPath = path.join(__dirname, '../client/dist');
app.use(express.static(clientDistPath));
app.get('*', (req, res) => {
  res.sendFile(path.join(clientDistPath, 'index.html'));
});

app.use((err, req, res, next) => {
  logger.error('Server error:', err);
  res.status(500).json({ error: 'Internal server error', message: err.message });
});

async function startServer() {
  try {
    initDatabase();
    initScheduler();
    app.listen(PORT, () => {
      logger.info(`Server running on port ${PORT}`);
    });
  } catch (error) {
    logger.error('Failed to start server:', error);
    process.exit(1);
  }
}

startServer();