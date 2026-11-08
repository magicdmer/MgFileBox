# MgFileBox

MgFileBox 是一个面向个人 VPS 部署的私有文件分享站。它提供管理员登录、文件上传、分享链接、提取码、分享有效期和分享管理能力，适合用于个人临时传文件、向指定对象分享文件或搭建轻量私有网盘入口。文件过期后保留，由管理员手动删除。

## 预览图
[预览页面](PREVIEW.md)

<img width="800" height="500" alt="file_upload" src="https://github.com/user-attachments/assets/44f8a464-2040-4012-b497-917617845df8" />

## 功能特性

- 管理员密码登录
- 单文件或多文件分享
- 分享链接自动生成
- 单条分享访问密码
- 带提取码的直达分享链接
- 分享过期时间，支持永不过期
- 文件管理页，支持查看分享中、已取消、已过期和已删除的记录
- 一键取消分享并保留文件；重新设置说明、提取码和有效期后生成全新链接，旧链接保持失效
- 管理员可直接下载未删除的文件，包括已过期分享中的文件
- 管理页支持更新软件安装包等文件，保留原分享链接、提取码、有效期和说明；多文件分享可逐个替换
- 软删除分享，并移除本地文件
- 分享到期后禁止访问并保留文件，由管理员手动删除
- 定时清理过期登录会话
- 手机和 PC 自适应页面

## 技术栈

- Go 1.25
- Gin
- SQLite
- Go HTML Template
- 原生 CSS 和 JavaScript

## 快速开始

1. 下载对应平台的发布包并解压

```bash
tar -xzf mgfilebox-vX.Y.Z-linux-amd64.tar.gz
cd mgfilebox
```

Windows 用户可下载 `.zip` 发布包，解压后进入 `mgfilebox` 目录。

2. 复制并编辑配置文件

```bash
cp .env.example .env
```

至少设置以下变量：

```env
ADMIN_PASSWORD=你的管理员密码
COOKIE_SECRET=一串足够长的随机字符串
APP_BASE_URL=https://你的域名
```

3. 运行

```bash
chmod +x ./mgfilebox
./mgfilebox
```

Windows 下运行：

```powershell
.\mgfilebox.exe
```

4. 打开浏览器访问

```text
http://localhost:8080/login
```

注意：程序启动时会自动读取运行目录下的 `.env` 文件。系统环境变量优先级高于 `.env`，生产环境也可以通过 systemd、Docker 或进程管理工具配置环境变量。

发布包内必须保留 `web/templates` 和 `web/static`。当前程序使用相对路径加载模板、静态资源和默认 `./data`，建议在解压后的 `mgfilebox` 目录内运行可执行文件。

## 配置说明

配置项来自环境变量或运行目录下的 `.env` 文件，可参考 `.env.example`。如果两边同时设置，系统环境变量优先。

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `PORT` | HTTP 监听端口 | `8080` |
| `APP_BASE_URL` | 对外分享链接基础地址 | `http://localhost:8080` |
| `ADMIN_PASSWORD` | 明文管理员密码，启动时转换为 bcrypt hash | 无 |
| `ADMIN_PASSWORD_HASH` | 已生成的 bcrypt 管理员密码 hash | 无 |
| `COOKIE_SECRET` | Cookie 签名和提取码加密密钥 | 空时随机生成 |
| `DATA_DIR` | 数据根目录 | `./data` |
| `UPLOAD_DIR` | 上传文件目录 | `./data/uploads` |
| `DB_PATH` | SQLite 数据库路径 | `./data/app.db` |
| `SESSION_TTL_HOURS` | 管理员会话有效小时数 | `24` |
| `CLEANUP_INTERVAL_MINUTES` | 过期登录会话及替换后旧版残留文件的清理间隔（分钟），设为 `0` 关闭；不清理当前分享文件 | `30` |
| `MAX_UPLOAD_SIZE_MB` | 每次上传的文件总大小上限（MB，正整数），多文件合计计算 | `512` |

生产环境请固定设置 `COOKIE_SECRET`，否则重启后已有提取码链接和解锁 Cookie 可能失效。

## 开发者

源码构建和测试命令：

```bash
# 编译
mkdir -p bin
go build -o ./bin/mgfilebox ./cmd/server

# 运行
./bin/mgfilebox

# 测试
go test ./...
```

Windows 下如果需要生成 `.exe`，可使用 `go build -o ./bin/mgfilebox.exe ./cmd/server`，运行时执行 `.\bin\mgfilebox.exe`。

查看版本：

```bash
./bin/mgfilebox --version
```

发布构建时可通过 ldflags 注入版本号：

```bash
go build -ldflags "-X main.version=0.1.0" -o ./bin/mgfilebox ./cmd/server
```

更详细的工程开发说明见 [docs/development-guide.md](docs/development-guide.md)。

## 部署建议

- 使用 Nginx 或 Caddy 做 HTTPS 反向代理。
- 使用 systemd、Supervisor 或其他进程管理工具守护服务。
- 生产环境建议配置独立数据目录，例如 `/opt/mgfilebox/data`。
- 生产环境务必设置 `APP_BASE_URL`、`COOKIE_SECRET`，并设置 `ADMIN_PASSWORD` 或 `ADMIN_PASSWORD_HASH`。
- 建议定期备份 SQLite 数据库和上传文件目录。

## 项目结构

```text
cmd/server/                 服务入口
internal/config/            配置加载
internal/models/            领域模型
internal/repository/        SQLite 数据访问
internal/security/          密码、签名、加密和随机值
internal/service/           核心业务逻辑
internal/web/               Gin 路由和 HTTP handler
web/templates/              HTML 模板
web/static/                 静态资源
docs/                       开发文档
```
