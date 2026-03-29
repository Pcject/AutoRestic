FROM node:18-alpine

# 安装 restic
RUN apk add --no-cache restic

WORKDIR /app

# 设置环境变量
ENV NODE_ENV=production
ENV JWT_SECRET=autorestic-secret-key-change-in-production
ENV PORT=3001

# 复制后端文件
COPY package*.json ./
RUN npm install --production
COPY server ./server

# 复制前端构建文件
COPY client/dist ./client/dist

# 创建数据目录
RUN mkdir -p data logs

EXPOSE 3001

CMD ["node", "server/index.js"]
