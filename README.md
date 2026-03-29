# AutoRestic

基于 restic 的自动备份服务，支持通过网页界面管理备份任务。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Node.js Version](https://img.shields.io/badge/node-%3E%3D16-brightgreen.svg)](https://nodejs.org)
[![Docker](https://img.shields.io/badge/docker-supported-blue.svg)](https://www.docker.com)

## ✨ 功能特性

- 📦 完整的 restic CLI 功能支持
- 🎨 美观的可视化备份管理界面
- ☁️ 多种存储后端支持（本地、SFTP、S3、B2、Azure、GCS、REST）
- ⏰ 定时自动备份（Cron 表达式）
- 📊 备份日志和监控
- 🔐 用户认证系统
- 📸 备份快照管理
- 🧹 仓库健康检查和清理

## 🛠️ 技术栈

- **后端**：Node.js + Express
- **前端**：React + Material-UI
- **数据库**：SQLite
- **任务调度**：node-cron
- **备份引擎**：restic

---

## 🚀 快速开始（Docker Compose）

这是最简单和推荐的部署方式，只需几分钟即可启动服务。

### 前置要求

- Docker 和 Docker Compose 已安装
- 至少 1GB 可用内存
- 足够的磁盘空间用于备份数据

### 1. 克隆项目

```bash
git clone <repository-url>
cd AutoRestic
```

### 2. 配置环境变量（重要！）

首先，复制示例配置文件：

```bash
cp .env.example .env
```

然后，生成一个安全的 JWT 密钥：

```bash
# 方法1：使用 openssl 生成（推荐）
JWT_SECRET=$(openssl rand -base64 32)
echo "JWT_SECRET=$JWT_SECRET" >> .env

# 方法2：使用 node.js 生成
JWT_SECRET=$(node -e "console.log(require('crypto').randomBytes(32).toString('base64'))")
echo "JWT_SECRET=$JWT_SECRET" >> .env
```

或者手动编辑 `.env` 文件，修改以下配置：

```env
# JWT 密钥（生产环境必须修改为强随机密钥）
JWT_SECRET=your-strong-random-secret-key-here

# 服务端口（可选，默认 3001）
PORT=3001

# 日志级别（可选）
LOG_LEVEL=info
```

> ⚠️ **安全警告**：生产环境请务必修改 `JWT_SECRET`！使用默认密钥会导致严重的安全风险！

### 3. 启动服务

```bash
# 使用 Docker Compose 一键启动
docker-compose up -d
```

第一次启动会自动完成以下步骤：
- 构建 Docker 镜像（包含 restic 备份工具）
- 创建必要的数据目录（`./data`、`./logs`）
- 初始化 SQLite 数据库
- 创建默认管理员账号
- 启动后台服务

### 4. 检查服务状态

```bash
# 查看容器状态
docker-compose ps

# 查看服务日志
docker-compose logs -f autorestic
```

等待健康检查通过后，服务就可以访问了。

### 5. 访问应用

打开浏览器访问：**http://localhost:3001**

### 6. 首次登录

使用默认账号登录：

- **用户名**：`admin`
- **密码**：`admin123`

> 🔐 **重要**：首次登录后请立即修改默认密码！

---

## 📋 常用 Docker Compose 命令

```bash
# 启动服务
docker-compose up -d

# 停止服务
docker-compose down

# 重启服务
docker-compose restart

# 查看服务状态
docker-compose ps

# 查看实时日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f autorestic

# 重新构建镜像并启动
docker-compose up -d --build

# 进入容器进行调试
docker-compose exec autorestic sh

# 删除容器和数据（谨慎使用！）
docker-compose down -v
```

---

## 📖 详细安装指南

### Docker Compose 部署（推荐）

#### 配置说明

`docker-compose.yml` 包含完整的生产环境配置：

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
      - ./data:/app/data          # SQLite 数据库和数据文件
      - ./logs:/app/logs          # 应用日志
    environment:
      - NODE_ENV=production
      - JWT_SECRET=autorestic-secret-key-change-in-production
      - PORT=3001
    restart: unless-stopped        # 自动重启
    healthcheck:                    # 健康检查
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3001"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

#### 数据持久化

| 目录 | 说明 |
|------|------|
| `./data` | 存放 SQLite 数据库和应用数据 |
| `./logs` | 存放应用日志文件 |

建议定期备份 `./data` 目录以防止数据丢失。

#### 环境变量参考

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `NODE_ENV` | 运行环境 | `production` | 否 |
| `PORT` | 服务端口 | `3001` | 否 |
| `JWT_SECRET` | JWT 签名密钥 | - | **是** |
| `RESTIC_PATH` | restic 可执行文件路径 | `/usr/local/bin/restic` | 否 |
| `DATABASE_PATH` | SQLite 数据库路径 | `./data/autorestic.db` | 否 |
| `LOG_LEVEL` | 日志级别 (debug/info/warn/error) | `info` | 否 |

### 手动 Docker 部署

如果需要更灵活的容器管理：

```bash
# 1. 构建镜像
docker build -t autorestic:latest .

# 2. 创建数据目录
mkdir -p data logs

# 3. 生成 JWT 密钥
JWT_SECRET=$(openssl rand -base64 32)

# 4. 运行容器
docker run -d \
  --name autorestic \
  -p 3001:3001 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  -e JWT_SECRET=$JWT_SECRET \
  --restart unless-stopped \
  autorestic:latest

# 5. 查看日志
docker logs -f autorestic
```

### 本地开发部署

详见 [本地开发指南](#本地开发)。

---

## 📖 使用指南

### 创建仓库

1. 进入"仓库管理"页面
2. 点击"新建仓库"
3. 填写仓库信息：
   - **名称**：仓库名称
   - **类型**：选择存储后端类型
   - **URL**：仓库地址
   - **密码**：仓库密码
   - **环境变量**：云存储认证信息（JSON格式）
4. 点击"初始化"按钮初始化仓库

> 💡 提示：如果仓库已存在，系统会自动检测并跳过初始化步骤。

### 支持的存储后端

| 类型 | URL 格式 | 说明 |
|------|----------|------|
| 本地存储 | `/path/to/repo` | 本地文件系统 |
| SFTP | `sftp:user@host:/path/to/repo` | SFTP# AutoRestic
