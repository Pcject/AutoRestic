# AutoRestic

An automatic backup service based on restic, with a web interface for managing backup tasks.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Node.js Version](https://img.shields.io/badge/node-%3E%3D16-brightgreen.svg)](https://nodejs.org)
[![Docker](https://img.shields.io/badge/docker-supported-blue.svg)](https://www.docker.com)
[![GitHub Repo](https://img.shields.io/badge/GitHub-Pcject%2FAutoRestic-blue?logo=github)](https://github.com/Pcject/AutoRestic)
[![Docker Image](https://img.shields.io/badge/Docker-pcject%2Fauto--restic-blue?logo=docker)](https://hub.docker.com/r/pcject/auto-restic)

---

## ⚠️ Disclaimer

This project was developed with assistance from **Trae IDE + doubao-seed-2.0-code**. It **has not been fully tested** and is provided for **learning and exchange purposes only**. Do not use directly in production environments. Use at your own risk.

---

## ✨ Features

- 📦 Full restic CLI functionality support
- 🎨 Beautiful visual backup management interface
- ☁️ Multiple storage backend support (Local, SFTP, S3, B2, Azure, GCS, REST)
- ⏰ Scheduled automatic backups (Cron expressions)
- 📊 Backup logs and monitoring
- 🔐 User authentication system
- 📸 Backup snapshot management
- 🧹 Repository health checks and pruning

## 🛠️ Tech Stack

- **Backend**: Node.js + Express
- **Frontend**: React + Material-UI
- **Database**: SQLite
- **Task Scheduling**: node-cron
- **Backup Engine**: restic

---

## 🚀 Quick Start (Docker Compose)

This is the simplest and recommended deployment method, getting you up and running in minutes.

### Prerequisites

- Docker and Docker Compose installed
- At least 1GB available RAM
- Sufficient disk space for backup data

### 1. Clone the Repository

```bash
git clone git@github.com:Pcject/AutoRestic.git
cd AutoRestic
```

### 2. Configure Environment Variables (Important!)

First, copy the example configuration file:

```bash
cp .env.example .env
```

Then, generate a secure JWT secret:

```bash
# Method 1: Generate using openssl (recommended)
JWT_SECRET=$(openssl rand -base64 32)
echo "JWT_SECRET=$JWT_SECRET" >> .env

# Method 2: Generate using node.js
JWT_SECRET=$(node -e "console.log(require('crypto').randomBytes(32).toString('base64'))")
echo "JWT_SECRET=$JWT_SECRET" >> .env
```

Or manually edit the `.env` file and modify the following configuration:

```env
# JWT secret (must change to a strong random key in production)
JWT_SECRET=your-strong-random-secret-key-here

# Service port (optional, default 3001)
PORT=3001

# Log level (optional)
LOG_LEVEL=info
```

> ⚠️ **Security Warning**: Always change `JWT_SECRET` in production! Using the default secret poses serious security risks!

### 3. Start the Service

```bash
# Start with Docker Compose
docker-compose up -d
```

First startup will automatically:
- Build the Docker image (including restic backup tool)
- Create necessary data directories (`./data`, `./logs`)
- Initialize SQLite database
- Create default admin account
- Start background service

### 4. Check Service Status

```bash
# View container status
docker-compose ps

# View service logs
docker-compose logs -f autorestic
```

Wait for health check to pass, then the service is accessible.

### 5. Access the Application

Open browser and visit: **http://localhost:3001**

### 6. First Login

Use default credentials to log in:

- **Username**: `admin`
- **Password**: `admin123`

> 🔐 **Important**: Change the default password immediately after first login!

---

## 📋 Common Docker Compose Commands

```bash
# Start service
docker-compose up -d

# Stop service
docker-compose down

# Restart service
docker-compose restart

# View service status
docker-compose ps

# View real-time logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f autorestic

# Rebuild image and start
docker-compose up -d --build

# Enter container for debugging
docker-compose exec autorestic sh

# Delete container and data (use with caution!)
docker-compose down -v
```

---

## 📖 Detailed Installation Guide

### Docker Compose Deployment (Recommended)

#### Configuration说明

`docker-compose.yml` contains complete production environment configuration:

```yaml
version: '3.8'

services:
  autorestic:
    image: autorestic:latest
    build: .
    container_name: autorestic
    ports:
      - "3001:3001"
    volumes:
      - ./data:/app/data          # SQLite database and data files
      - ./logs:/app/logs          # Application logs
    environment:
      - NODE_ENV=production
      - JWT_SECRET=autorestic-secret-key-change-in-production
      - PORT=3001
    restart: unless-stopped        # Auto restart
    healthcheck:                    # Health check
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3001"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

#### Data Persistence

| Directory | Description |
|-----------|-------------|
| `./data` | Stores SQLite database and application data |
| `./logs` | Stores application log files |

Regularly backup the `./data` directory to prevent data loss.

#### Environment Variable Reference

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `NODE_ENV` | Runtime environment | `production` | No |
| `PORT` | Service port | `3001` | No |
| `JWT_SECRET` | JWT signing secret | - | **Yes** |
| `RESTIC_PATH` | restic executable path | `/usr/local/bin/restic` | No |
| `DATABASE_PATH` | SQLite database path | `./data/autorestic.db` | No |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | `info` | No |

### Manual Docker Deployment

For more flexible container management:

```bash
# 1. Build image
docker build -t autorestic:latest .

# 2. Create data directories
mkdir -p data logs

# 3. Generate JWT secret
JWT_SECRET=$(openssl rand -base64 32)

# 4. Run container
docker run -d \
  --name autorestic \
  -p 3001:3001 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  -e JWT_SECRET=$JWT_SECRET \
  --restart unless-stopped \
  autorestic:latest

# 5. View logs
docker logs -f autorestic
```

### Local Development Deployment

See [Local Development Guide](#local-development).

---

## 📖 Usage Guide

### Create a Repository

1. Go to "Repository Management" page
2. Click "New Repository"
3. Fill in repository information:
   - **Name**: Repository name
   - **Type**: Select storage backend type
   - **URL**: Repository address
   - **Password**: Repository password
   - **Environment Variables**: Cloud storage auth info (JSON format)
4. Click "Initialize" button to initialize the repository

> 💡 Tip: If the repository already exists, the system will automatically detect and skip the initialization step.

### Supported Storage Backends

| Type | URL Format | Description |
|------|------------|-------------|
| Local Storage | `/path/to/repo` | Local filesystem |
| SFTP | `sftp:user@host:/path/to/repo` | SFTP server |
| S3 | `s3:bucket/path/to/repo` | Amazon S3 |
| B2 | `b2:bucket/path/to/repo` | Backblaze B2 |
| Azure | `azure:container/path/to/repo` | Azure Blob Storage |
| GCS | `gs:bucket/path/to/repo` | Google Cloud Storage |
| REST | `rest:http://host:port/path/to/repo` | REST server |

### Create a Backup Task

1. Go to "Backup Management" page
2. Click "New Backup"
3. Fill in backup information:
   - **Name**: Backup task name
   - **Select Repository**: Select an existing repository
   - **Source Paths**: Paths to backup (one per line)
   - **Exclude Patterns**: File patterns to exclude (optional)
   - **Tags**: Backup tags (optional)
4. After saving, you can click "Backup Now" to execute immediately

### Set Up Scheduled Tasks

1. Go to "Scheduled Tasks" page
2. Click "New Scheduled Task"
3. Fill in schedule information:
   - Select backup task
   - Cron expression: Define execution time
   - Enable/disable task
4. After saving, the task will execute automatically

### Cron Expression Examples

| Expression | Description |
|------------|-------------|
| `0 2 * * *` | Every day at 2 AM |
| `0 12 * * *` | Every day at 12 PM |
| `0 3 * * 0` | Every Sunday at 3 AM |
| `0 2 1 * *` | 1st day of every month at 2 AM |
| `0 */6 * * *` | Every 6 hours |
| `0 0 * * *` | Every midnight |

Format: `minute hour day month weekday`

### View Logs

1. Go to "Log Records" page
2. View execution logs of all backup tasks
3. Support filtering by status and operation type
4. Support filtering cached messages
5. Terminal-style detailed output display

---

## 💻 Local Development

### Prerequisites

- Node.js >= 16
- npm >= 8
- restic >= 0.15

### Installation Steps

1. Clone the repository
```bash
git clone git@github.com:Pcject/AutoRestic.git
cd AutoRestic
```

2. Install dependencies
```bash
# Install backend dependencies
npm install

# Install frontend dependencies
cd client
npm install
cd ..
```

3. Configure environment variables
```bash
cp .env.example .env
# Edit .env file, set JWT_SECRET
```

4. Start development servers
```bash
# Terminal 1: Start backend
npm run dev

# Terminal 2: Start frontend
cd client
npm run dev
```

5. Access the application
- Frontend: http://localhost:5173
- Backend: http://localhost:3001

---

## 📚 API Documentation

### Authentication

- `POST /api/auth/login` - User login
- `POST /api/auth/register` - User registration
- `GET /api/auth/me` - Get current user info

### Repository Management

- `GET /api/repositories` - Get repository list
- `POST /api/repositories` - Create repository
- `GET /api/repositories/:id` - Get repository details
- `PUT /api/repositories/:id` - Update repository
- `DELETE /api/repositories/:id` - Delete repository
- `POST /api/repositories/:id/init` - Initialize repository
- `POST /api/repositories/:id/check` - Check repository
- `POST /api/repositories/:id/prune` - Prune repository
- `GET /api/repositories/:id/stats` - Get repository stats

### Backup Management

- `GET /api/backups` - Get backup list
- `POST /api/backups` - Create backup
- `GET /api/backups/:id` - Get backup details
- `PUT /api/backups/:id` - Update backup
- `DELETE /api/backups/:id` - Delete backup
- `POST /api/backups/:id/run` - Run backup
- `GET /api/backups/:id/snapshots` - Get snapshot list
- `GET /api/backups/:id/logs` - Get backup logs

### Scheduled Tasks

- `GET /api/schedules` - Get schedule list
- `POST /api/schedules` - Create schedule
- `GET /api/schedules/:id` - Get schedule details
- `PUT /api/schedules/:id` - Update schedule
- `DELETE /api/schedules/:id` - Delete schedule
- `POST /api/schedules/:id/toggle` - Toggle schedule status

### Task Management

- `GET /api/tasks` - Get task list
- `GET /api/tasks/:id` - Get task details
- `DELETE /api/tasks/:id` - Delete task

### Logs

- `GET /api/logs` - Get log list
- `GET /api/logs/stats` - Get log stats
- `GET /api/logs/:id` - Get log details

---

## 🔒 Security Recommendations

1. **Change default password**: Change admin password immediately after first login
2. **Use strong JWT secret**: Use randomly generated strong secret in production
3. **Protect repository password**: Use strong password for restic repository
4. **Regularly backup database**: Regularly backup SQLite database file
5. **Use HTTPS**: Recommended to use HTTPS deployment in production
6. **Restrict network access**: Configure firewall to restrict access
7. **Regularly update**: Regularly update restic and application versions
8. **Don't expose default JWT_SECRET**: Never use default JWT secret

---

## 🛟 Troubleshooting

### Backup Failed

1. Check if repository is initialized
2. Verify repository password is correct
3. Check if source paths exist
4. View logs for detailed error information

### Scheduled Task Not Executing

1. Confirm task is enabled
2. Check if Cron expression is correct
3. View backend logs

### Repository Connection Failed

1. Check network connection
2. Verify environment variable configuration
3. Confirm storage service is available

### Docker Container Issues

```bash
# View container status
docker-compose ps

# View container logs
docker-compose logs -f

# Restart container
docker-compose restart

# Rebuild
docker-compose up -d --build
```

### JWT Secret Error

If you see "secretOrPrivateKey must have a value" error:

```bash
# Ensure JWT_SECRET is set in .env file
echo "JWT_SECRET=$(openssl rand -base64 32)" >> .env
# Then restart service
docker-compose restart
```

---

## 🤝 Contributing

Issues and Pull Requests are welcome!

---

## 📄 License

MIT License
