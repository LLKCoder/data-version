# Data Vision

一个面向业务指标的 Grafana 风格数据看板应用：

- Vue 3 + TypeScript + TailwindCSS + Apache ECharts
- Golang + Gin + GORM
- MySQL 8.4
- Docker Compose 一键启动
- Chromium/Playwright PDF 导出服务
- MySQL、PostgreSQL、SQLite 和 HTTP API 数据源
- SQL、HTTP 和跨数据源 Join/计算查询
- 可拖拽看板编辑、表数据分页预览、JSON 导入导出

## Docker Compose 启动

```bash
copy .env.example .env
docker compose up --build
```

打开 <http://localhost>。默认会自动创建 `ops-overview` 示例看板。

## 本地开发

先启动数据库：

```bash
docker compose up mysql
```

然后分别启动前后端：

```bash
cd backend
go run ./cmd/server
```

```bash
cd frontend
npm install
npm run dev
```

本地后端默认监听 `18085`，数据库默认使用 `127.0.0.1:3306`，可通过 `PORT` 和 `DATABASE_DSN` 覆盖；前端 Vite 会将 `/api` 代理到 `http://localhost:18085`。
数据源凭据使用 `DATASOURCE_ENCRYPTION_KEY` 加密保存；生产环境必须替换示例密钥。
Docker 构建已启用 CGO 以支持 SQLite；本地运行 SQLite 测试需要安装 C 编译器并设置 `CGO_ENABLED=1`。

## 当前 API

```text
GET    /api/v1/health
GET    /api/v1/dashboards
POST   /api/v1/dashboards
GET    /api/v1/dashboards/:uid
PUT    /api/v1/dashboards/:uid
DELETE /api/v1/dashboards/:uid
POST   /api/v1/dashboards/:uid/export/pdf
GET    /api/v1/dashboards/:uid/export
POST   /api/v1/dashboards/import
GET    /api/v1/datasources
POST   /api/v1/datasources
PUT    /api/v1/datasources/:uid
DELETE /api/v1/datasources/:uid
POST   /api/v1/datasources/:uid/test
GET    /api/v1/datasources/:uid/tables
GET    /api/v1/datasources/:uid/table-preview
POST   /api/v1/query/execute
```

## 后续迭代

当前版本暂不包含认证与权限、变量系统、查询缓存、告警和实时推送。
