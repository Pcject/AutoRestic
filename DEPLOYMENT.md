# 部署指南

## 系统要求

- Node.js >= 16
- npm >= 8
- restic >= 0.15
- 至少 1GB 可用内存
- 足够的磁盘空间用于备份数据

## 开发环境部署

### 1. 克隆项目

```bash
git clone <repository-url>
cd AutoRestic
```

### 2. 安装依赖

```bash
npm install
cd client
npm install
cd ..
```

### 3. 配置环境变量

```bash
cp .env.example .env
```

编辑 `.env` 文件，设置必要的配置项。

### 4. 启动开发服务器

```bash
# 终端1：启动后端
npm run dev

# 终端2：启动前端
cd client
npm run dev
```

访问 http://localhost:5173

## 生产环境部署

### 方案一：直接部署

#### 1. 构建前端

```bash
npm run build
```

#### 2. 配置生产环境变量

```bash
NODE_ENV=production
PORT=3001
JWT_SECRET=<strong-random-secret>
RESTIC_PATH=/usr/local/bin/restic
DATABASE_PATH=/var/lib/autorestic/autorestic.db
LOG_LEVEL=info
```

#### 3. 创建必要的目录

```bash
sudo mkdir -p /var/lib/autorestic
sudo mkdir -p /var/log/autorestic
sudo chown -R $USER:$USER /var/lib/autorestic
sudo chown -R $USER:$USER /var/log/autorestic
```

#### 4. 使用 PM2 管理进程

```bash
# 安装 PM2
npm install -g pm2

# 启动应用
pm2 start server/index.js --name autorestic --env production

# 设置开机自启
pm2 startup
pm2 save

# 查看状态
pm2 status

# 查看日志
pm2 logs autorestic
```

### 方案二：Docker 部署

#### 1. 创建 Dockerfile

项目根目录已包含 Dockerfile。

#### 2. 构建镜像

```bash
docker build -t autorestic:latest .
```

#### 3. 运行容器

```bash
docker run -d \
  --name autorestic \
  -p 3001:3001 \
  -v /path/to/data:/app/data \
  -v /path/to/logs:/app/logs \
  -e NODE_ENV=production \
  -e JWT_SECRET=your-secret-key \
  autorestic:latest
```

#### 4. 使用 Docker Compose

创建 `docker-compose.yml`：

```yaml
version: '3.8'

services:
  autorestic:
    build: .
    ports:
      - "3001:3001"
    volumes:
      - ./data:/app/data
      - ./logs:/app/logs
      - /path/to/backup/source:/backup/source:ro
    environment:
      - NODE_ENV=production
      - JWT_SECRET=your-secret-key
      - RESTIC_PATH=/usr/local/bin/restic
    restart: unless-stopped
```

启动：

```bash
docker-compose up -d
```

### 方案三：Nginx 反向代理

#### 1. 配置 Nginx

创建 `/etc/nginx/sites-available/autorestic`：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:3001;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }
}
```

#### 2. 启用配置

```bash
sudo ln -s /etc/nginx/sites-available/autorestic /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

#### 3. 配置 HTTPS（使用 Let's Encrypt）

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```

## 安全加固

### 1. 修改默认密码

首次登录后立即修改管理员密码。

### 2. 配置防火墙

```bash
# UFW (Ubuntu/Debian)
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable

# firewalld (CentOS/RHEL)
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

### 3. 限制文件权限

```bash
chmod 600 .env
chmod 700 data
chmod 700 logs
```

### 4. 定期更新

```bash
# 更新系统包
sudo apt update && sudo apt upgrade -y

# 更新 restic
brew upgrade restic  # macOS
# 或重新下载最新版本
```

## 监控和维护

### 1. 日志管理

```bash
# 查看应用日志
pm2 logs autorestic

# 查看错误日志
tail -f /var/log/autorestic/error.log

# 日志轮转配置
sudo nano /etc/logrotate.d/autorestic
```

`/etc/logrotate.d/autorestic` 内容：

```
/var/log/autorestic/*.log {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    create 0640 $USER $USER
    sharedscripts
}
```

### 2. 数据库备份

```bash
# 创建备份脚本
cat > /usr/local/bin/backup-autorestic-db.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/backup/autorestic"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR
cp /var/lib/autorestic/autorestic.db $BACKUP_DIR/autorestic_$DATE.db
find $BACKUP_DIR -name "autorestic_*.db" -mtime +7 -delete
EOF

chmod +x /usr/local/bin/backup-autorestic-db.sh

# 添加到 crontab
crontab -e
# 添加：0 2 * * * /usr/local/bin/backup-autorestic-db.sh
```

### 3. 性能监控

使用 PM2 监控：

```bash
pm2 monit
```

或使用第三方监控工具：
- Prometheus + Grafana
- Datadog
- New Relic

## 故障恢复

### 1. 恢复数据库

```bash
# 停止服务
pm2 stop autorestic

# 恢复数据库
cp /backup/autorestic/autorestic_YYYYMMDD_HHMMSS.db /var/lib/autorestic/autorestic.db

# 启动服务
pm2 start autorestic
```

### 2. 恢复备份数据

使用 restic CLI 恢复：

```bash
export RESTIC_REPOSITORY=/path/to/repo
export RESTIC_PASSWORD=your-password

# 列出快照
restic snapshots

# 恢复快照
restic restore <snapshot-id> --target /restore/path
```

## 升级指南

### 1. 备份数据

```bash
# 备份数据库
cp data/autorestic.db data/autorestic.db.backup

# 备份配置
cp .env .env.backup
```

### 2. 更新代码

```bash
git pull origin main
npm install
cd client
npm install
npm run build
cd ..
```

### 3. 重启服务

```bash
pm2 restart autorestic
```

## 常见问题

### Q: 如何修改端口？

A: 修改 `.env` 文件中的 `PORT` 变量。

### Q: 如何增加备份存储空间？

A: 扩展存储后端的容量，或添加新的仓库。

### Q: 如何迁移到新服务器？

A: 
1. 备份数据库和配置文件
2. 在新服务器上部署应用
3. 恢复数据库和配置
4. 更新 DNS 或负载均衡配置

### Q: 如何优化备份性能？

A:
- 使用 SSD 存储
- 增加并发备份任务
- 使用增量备份
- 优化网络带宽

## 支持

如有问题，请提交 Issue 或联系技术支持。