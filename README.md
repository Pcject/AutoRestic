# AutoRestic

基于 restic 的自动备份服务，支持通过网页界面管理备份任务。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Node.js Version](https://img.shields.io/badge/node-%3E%3D16-brightgreen.svg)](https://nodejs.org)
[![Docker](https://img.shields.io/badge/docker-supported-blue.svg)](https://www.docker.com)
[![GitHub Repo](https://img.shields.io/badge/GitHub-Pcject%2FAutoRestic-blue?logo=github)](https://github.com/Pcject/AutoRestic)
[![Docker Image](https://img.shields.io/badge/Docker-pcject%2Fauto--restic-blue?logo=docker)](https://hub.docker.com/r/pcject/auto-restic)

---

## ⚠️ 免责声明

本项目由 **Trae IDE + doubao-seed-2.0-code** 辅助开发，**未经全面测试**，仅供**交流学习**使用。请勿在生产环境中直接使用，使用时请自行承担风险。

---

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
git clone git@github.com:Pcject/AutoRestic.git
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
| SFTP | `sftp:user@host:/path/to/repo` | SFTP 服务器 |
| S3 | `s3:bucket/path/to/repo` | Amazon S3 |
| B2 | `b2:bucket/path/to/repo` | Backblaze B2 |
| Azure | `azure:container/path/to/repo` | Azure Blob Storage |
| GCS | `gs:bucket/path/to/repo` | Google Cloud Storage |
| REST | `rest:http://host:port/path/to/repo` | REST 服务器 |

### 创建备份任务

1. 进入"备份管理"页面
2. 点击"新建备份"
3. 填写备份信息：
   - **名称**：备份任务名称
   - **选择仓库**：选择已创建的仓库
   - **源路径**：需要备份的路径（每行一个）
   - **排除模式**：排除的文件模式（可选）
   - **标签**：备份标签（可选）
4. 保存后可以点击"立即备份"执行备份

### 设置定时任务

1. 进入"定时任务"页面
2. 点击"新建定时任务"
3. 填写定时信息：
   - 选择备份任务
   - Cron 表达式：定义执行时间
   - 启用/禁用任务
4. 保存后任务会自动执行

### Cron 表达式示例

| 表达式 | 说明 |
|---------|------|
| `0 2 * * *` | 每天凌晨 2 点 |
| `0 12 * * *` | 每天中午 12 点 |
| `0 3 * * 0` | 每周日凌晨 3 点 |
| `0 2 1 * *` | 每月 1 号凌晨 2 点 |
| `0 */6 * * *` | 每 6 小时 |
| `0 0 * * *` | 每天午夜 |

格式：`分 时 日 月 周`

### 查看日志

1. 进入"日志记录"页面
2. 可以查看所有备份任务的执行日志
3. 支持按状态和操作类型筛选
4. 支持过滤缓存消息
5. 终端风格的详细输出展示

---

## 💻 本地开发

### 前置要求

- Node.js >= 16
- npm >= 8
- restic >= 0.15

### 安装步骤

1. 克隆项目
```bash
git clone git@github.com:Pcject/AutoRestic.git
cd AutoRestic
```

2. 安装依赖
```bash
# 安装后端依赖
npm install

# 安装前端依赖
cd client
npm install
cd ..
```

3. 配置环境变量
```bash
cp .env.example .env
# 编辑 .env 文件，设置 JWT_SECRET
```

4. 启动开发服务器
```bash
# 终端1：启动后端
npm run dev

# 终端2：启动前端
cd client
npm run dev
```

5. 访问应用
- 前端地址：http://localhost:5173
- 后端地址：http://localhost:3001

---

## 📚 API 文档

### 认证

- `POST /api/auth/login` - 用户登录
- `POST /api/auth/register` - 用户注册
- `GET /api/auth/me` - 获取当前用户信息

### 仓库管理

- `GET /api/repositories` - 获取仓库列表
- `POST /api/repositories` - 创建仓库
- `GET /api/repositories/:id` - 获取仓库详情
- `PUT /api/repositories/:id` - 更新仓库
- `DELETE /api/repositories/:id` - 删除仓库
- `POST /api/repositories/:id/init` - 初始化仓库
- `POST /api/repositories/:id/check` - 检查仓库
- `POST /api/repositories/:id/prune` - 清理仓库
- `GET /api/repositories/:id/stats` - 获取仓库统计

### 备份管理

- `GET /api/backups` - 获取备份列表
- `POST /api/backups` - 创建备份
- `GET /api/backups/:id` - 获取备份详情
- `PUT /api/backups/:id` - 更新备份
- `DELETE /api/backups/:id` - 删除备份
- `POST /api/backups/:id/run` - 执行备份
- `GET /api/backups/:id/snapshots` - 获取快照列表
- `GET /api/backups/:id/logs` - 获取备份日志

### 定时任务

- `GET /api/schedules` - 获取定时任务列表
- `POST /api/schedules` - 创建定时任务
- `GET /api/schedules/:id` - 获取定时任务详情
- `PUT /api/schedules/:id` - 更新定时任务
- `DELETE /api/schedules/:id` - 删除定时任务
- `POST /api/schedules/:id/toggle` - 切换任务状态

### 任务管理

- `GET /api/tasks` - 获取任务列表
- `GET /api/tasks/:id` - 获取任务详情
- `DELETE /api/tasks/:id` - 删除任务

### 日志

- `GET /api/logs` - 获取日志列表
- `GET /api/logs/stats` - 获取日志统计
- `GET /api/logs/:id` - 获取日志详情

---

## 🔒 安全建议

1. **修改默认密码**：首次登录后立即修改管理员密码
2. **使用强 JWT 密钥**：生产环境使用随机生成的强密钥
3. **保护仓库密码**：使用强密码保护 restic 仓库
4. **定期备份数据库**：定期备份 SQLite 数据库文件
5. **使用 HTTPS**：生产环境建议使用 HTTPS 部署
6. **限制网络访问**：配置防火墙限制访问
7. **定期更新**：定期更新 restic 和应用版本
8. **不要暴露默认 JWT_SECRET**：永远不要使用默认的 JWT 密钥

---

## 🛟 故障排查

### 备份失败

1. 检查仓库是否已初始化
2. 验证仓库密码是否正确
3. 检查源路径是否存在
4. 查看日志获取详细错误信息

### 定时任务不执行

1. 确认任务已启用
2. 检查 Cron 表达式是否正确
3. 查看后端日志

### 仓库连接失败

1. 检查网络连接
2. 验证环境变量配置
3. 确认存储服务可用

### Docker 容器问题

```bash
# 查看容器状态
docker-compose ps

# 查看容器日志
docker-compose logs -f

# 重启容器
docker-compose restart

# 重新构建
docker-compose up -d --build
```

### JWT 密钥错误

如果看到 "secretOrPrivateKey must have a value" 错误：

```bash
# 确保在 .env 文件中设置了 JWT_SECRET
echo "JWT_SECRET=$(openssl rand -base64 32)" >> .env
# 然后重启服务
docker-compose restart
```

---

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

---

## 📄 许可证

MIT License
