# MgFileBox AI 开发文档

本文档面向后续参与本仓库开发的 AI 或自动化编码代理。开始修改前请先阅读本文，再按需阅读相关源码。

## 项目定位

MgFileBox 是一个部署在个人 VPS 上的私有文件分享站。当前功能包括：

- 管理员密码登录
- 文件上传并生成分享链接
- 单条分享访问密码和提取码直达链接
- 分享过期时间，支持永不过期
- 管理页查看分享中、已取消、已过期和已删除的记录，支持取消分享和重新分享
- 软删除分享并移除本地文件
- 分享过期后禁止访问并保留文件，由管理员手动删除；定时仅清理过期管理员会话

项目是单体 Go Web 服务，使用 Gin、SQLite 和服务端 HTML 模板，不包含前后端分离构建链。

## 技术栈

- 语言：Go 1.25
- Web 框架：`github.com/gin-gonic/gin`
- 数据库：SQLite，驱动为 `modernc.org/sqlite`
- 密码哈希：`golang.org/x/crypto/bcrypt`
- 前端：`web/templates/*.html` + `web/static/app.css` + `web/static/app.js`

## 目录结构

```text
cmd/server/main.go              服务入口、HTTP Server、优雅退出、清理 worker
internal/config/config.go       环境变量加载、默认值、目录初始化
internal/models/models.go       领域模型和简单状态判断
internal/repository/sqlite.go   SQLite 连接、内联迁移、CRUD
internal/security/security.go   密码、随机 token、签名、加解密
internal/service/service.go     核心业务逻辑、文件落盘、分享访问控制
internal/web/router.go          Gin 路由、handler、模板函数
web/templates/                  服务端 HTML 模板
web/static/                     CSS 和浏览器端 JS
docs/                           开发文档
```

## 运行和测试

本地构建和运行：

```bash
cp .env.example .env
# 编辑 .env，至少设置 ADMIN_PASSWORD、COOKIE_SECRET、APP_BASE_URL
mkdir -p bin
go build -o ./bin/mgfilebox ./cmd/server
./bin/mgfilebox
```

Windows 下如果需要生成 `.exe`，可使用 `go build -o ./bin/mgfilebox.exe ./cmd/server`，运行时执行 `.\bin\mgfilebox.exe`。`mkdir -p bin` 的作用是确保 `bin` 目录存在；在 PowerShell 中也可以用 `New-Item -ItemType Directory -Force bin` 达到同样效果。

可执行文件支持查看版本：

```bash
./bin/mgfilebox --version
```

`cmd/server/main.go` 中的 `version` 默认是 `dev`。发布构建时建议由 GitHub Actions 注入版本号：

```bash
go build -ldflags "-X main.version=0.1.0" -o ./bin/mgfilebox ./cmd/server
```

## 发布打包

GitHub Actions workflow 位于 `.github/workflows/release.yml`。

触发方式：

- 推送 tag：`v0.1.0` 这类 `v*` 标签会构建并创建 GitHub Release。
- 手动触发：`workflow_dispatch` 会构建包并上传为 workflow artifacts，适合测试 Actions。

手动触发参数：

- `version`：可选。用于测试包名和 `--version` 输出，例如 `0.1.0-test`；不填时使用 `dev-<commit>`。
- `create_release`：可选，默认 `false`。开启后会创建一个 `manual-v<version>` 的预发布 Release；普通测试不建议开启。

当前打包平台：

- `mgfilebox-vX.Y.Z-linux-amd64.tar.gz`
- `mgfilebox-vX.Y.Z-windows-amd64.zip`

发布包结构：

```text
mgfilebox/
├─ mgfilebox 或 mgfilebox.exe
├─ .env.example
├─ README.md
├─ LICENSE
├─ web/
│  ├─ templates/
│  └─ static/
└─ docs/
   └─ development-guide.md
```

发布包内必须保留 `web/templates` 和 `web/static`，程序运行时会从当前工作目录按相对路径加载模板和静态资源。

访问入口：

```text
http://localhost:8080/login
```

运行命令建议在项目根目录执行。当前程序使用相对路径加载 `web/templates`、`web/static` 以及默认 `./data`，如果切到 `bin` 目录后再运行可执行文件，模板、静态文件和默认数据目录都会相对 `bin` 解析，容易启动失败或写错数据位置。

运行测试：

```powershell
go test ./...
```

当前测试主要覆盖 `internal/service` 和 `internal/security`。修改业务规则、清理逻辑、密码/签名/加密逻辑时，应优先补充这些包的单元测试。

## 配置项

配置由系统环境变量或运行目录下的 `.env` 文件提供，见 `.env.example`。如果两边同时设置，系统环境变量优先：

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `PORT` | HTTP 监听端口 | `8080` |
| `APP_BASE_URL` | 生成分享 URL 的外部基础地址 | `http://localhost:8080` |
| `ADMIN_PASSWORD` | 明文管理员密码，启动时转 bcrypt hash | 无，必填二选一 |
| `ADMIN_PASSWORD_HASH` | 已生成的 bcrypt 管理员密码 hash | 无，必填二选一 |
| `COOKIE_SECRET` | cookie 签名和提取码加密密钥 | 空时随机生成，不适合生产 |
| `DATA_DIR` | 数据根目录 | `./data` |
| `UPLOAD_DIR` | 上传文件目录 | `./data/uploads` |
| `DB_PATH` | SQLite 数据库路径 | `./data/app.db` |
| `SESSION_TTL_HOURS` | 管理员会话有效小时数 | `24` |
| `CLEANUP_INTERVAL_MINUTES` | 过期管理员会话记录清理间隔（分钟），`0` 关闭；不清理分享文件 | `30` |
| `MAX_UPLOAD_SIZE_MB` | 每次上传的文件总大小上限（MB，正整数），多文件合计计算 | `512` |

注意：生产环境必须固定 `COOKIE_SECRET`。如果每次启动随机生成，已有分享的提取码加密值和解锁 cookie 会失效。

`.env` 在 `config.Load` 开始时自动读取，只会补充当前进程尚未设置的环境变量。这样本地和发布包可以直接使用 `.env`，Docker、systemd 或命令行传入的环境变量仍可覆盖 `.env`。

## 核心请求流程

管理员登录：

1. `POST /api/auth/login` 进入 `web.Server.handleLogin`。
2. `service.Login` 使用 bcrypt 校验管理员密码。
3. 登录成功后生成随机 token，只把 token 的 SHA-256 hash 存到 `admin_sessions`。
4. 浏览器保存 `mgbox_admin` HttpOnly cookie。
5. 后续后台页面由 `requireAdmin` 中间件调用 `ValidateAdminSession` 校验。

创建文件分享：

1. 管理员页面提交 multipart 表单到 `POST /api/shares/file`。
2. 上传页通过 `data-max-upload-size` 使用运行中服务的大小限制；上传框的上限标识仅在未选择文件时显示，选择文件后隐藏，清空后恢复。选择、拖入、移除文件时重新校验总大小，提交前再次校验，超限则显示自定义 Toast 且不发送文件，不再占用表单空间显示行内错误。Toast 不堆叠，约 4.5 秒后自动消失，支持手动关闭，鼠标悬停或键盘聚焦时暂停计时。请求体上限为配置的文件总大小加 1 MiB 表单开销，对已知 Content-Length 的超大请求立即拒绝，对分块上传则限制读取；超过请求体或文件总大小上限返回 413。multipart 内存阈值固定为 32 MiB，超过阈值的内容写入临时文件，请求处理结束后清理。
3. `web.parseCommonInput` 解析标题、说明、访问密码、过期小时数。`service.CreateFileShare` 合计检查文件大小后生成分享 ID，将文件保存到 `UPLOAD_DIR`。写入或关闭失败时，先关闭并删除当前半文件，再清理此前已保存的文件；清理失败也向调用方返回错误，不创建分享记录。
4. 如果设置访问密码，密码本身用 bcrypt hash 存储，同时使用 `COOKIE_SECRET` AES-GCM 加密明文提取码，用于生成带 `pickcode` 查询参数的管理页分享链接。
5. `repository.CreateShare` 在一个事务里写入 `shares` 和 `share_files`。

访问分享：

1. 公开页面为 `GET /s/:id`，此处 `id` 是 `shares.public_id`，与管理操作使用的稳定记录 ID 分离。旧数据首次迁移时将原 ID 作为公开 ID，保证现有有效链接兼容。
2. 分享不存在、已删除、已取消或已过期时渲染 `share.html` 的对应状态。
3. 无密码分享直接可下载。
4. 有密码分享需要提交 `POST /s/:id/unlock`，或使用管理页生成的 `?pickcode=` 链接。
5. 解锁成功后写入 `mgbox_unlock_<shareID>` HttpOnly cookie，cookie 内容是 HMAC 签名值。
6. 下载入口为 `GET /s/:id/download`，多文件分享通过 `?file=<fileID>` 指定文件。
7. 管理页下载入口为 `GET /api/admin/shares/:id/download`，必须有有效管理员会话；允许下载已过期或带提取码的分享文件，但拒绝已删除分享。多文件同样通过 `?file=<fileID>` 指定，文件必须属于该分享。公开下载入口继续校验过期时间和提取码。
8. 管理员调用 `POST /api/admin/shares/:id/cancel` 取消分享，只设置 `cancelled`，不删除文件；`GET /api/admin/shares/:id/settings` 读取说明、解密后的提取码和上次有效期，响应禁止缓存。
9. `POST /api/admin/shares/:id/reshare` 仅允许重新分享已取消或已过期、未删除且文件存在的记录。复用原文件及记录 ID，更新公开 ID、说明、提取码和有效期，不复制文件；公开入口仅接受最新公开 ID，解锁签名也绑定公开 ID。旧链接不会再次生效，过期会话清理和数据库重启不会恢复旧链接。
10. 取消或重新分享成功后，前端只更新当前文件记录的状态、图标、说明、提取码标识、有效期和复制链接，不刷新页面，以保留上传页正在进行的后台上传。

删除和清理：

1. 管理页删除调用 `POST /api/admin/shares/:id/delete`。
2. `service.DeleteShare` 逐个删除本地文件，文件已不存在也视为成功；全部成功后才对数据库执行软删除。失败时返回具体文件名和原因，保留原记录供管理员重试；多文件可能已部分删除。清除已删除记录前也会检查并删除历史残留文件，失败时保留记录。
3. 分享过期后仅禁止访问，文件和分享记录保留，直到管理员手动删除。
4. `cmd/server/main.go` 的 `runSessionCleanupWorker` 定时调用 `service.CleanupExpiredSessions`，只删除过期管理员会话记录；间隔设为 `0` 时不启动任务。

## 数据模型和数据库

SQLite 迁移写在 `internal/repository/sqlite.go` 的 `migrate` 方法中，启动时自动执行。

主要表：

- `shares`：分享主表，包含稳定记录 ID、可更换的公开 ID、取消状态、标题、说明、首个文件兼容字段、访问密码 hash、加密提取码、有效期时长、过期时间和删除时间。
- `share_files`：分享文件表，支持一个分享包含多个文件。
- `admin_sessions`：管理员会话，保存 token hash，不保存原始 token。
- `access_logs`：访问日志，目前记录查看和下载动作。

兼容性注意：

- `shares.file_name`、`shares.storage_path` 等字段仍保留，用于兼容早期单文件分享。
- `migrate` 中有把旧分享补写到 `share_files` 的语句。
- 新增字段时优先保持迁移幂等，参考 `ensureColumn`。

## 安全约定

- 管理员密码和分享访问密码都用 bcrypt，不要明文落库。
- 管理员 session token 只存 SHA-256 hash。
- 管理员登录按客户端 IP 做内存级失败限速：连续失败 5 次后锁定 15 分钟；进程重启后计数会清空。
- 分享解锁 cookie 由 `security.SignValue` 签名，校验入口是 `security.VerifySignedValue`。
- 提取码直达链接依赖 `PickcodeEncrypted`，加解密使用 `COOKIE_SECRET` 派生出的 AES-GCM key。
- 不要把 `COOKIE_SECRET`、管理员密码、上传文件内容写入日志。
- Cookie 均为 HttpOnly；当请求本身是 HTTPS 或 `X-Forwarded-Proto: https` 时会设置 `Secure`。

## 常见开发路径

新增后台页面或 API：

1. 在 `internal/web/router.go` 注册路由。
2. 如果需要登录保护，挂在 `admin := s.engine.Group("/")` 下。
3. 业务逻辑放到 `internal/service`，不要直接在 handler 里操作数据库或文件。
4. 数据访问放到 `internal/repository`。
5. 页面模板放到 `web/templates`，静态交互放到 `web/static/app.js`。

新增分享字段：

1. 先改 `internal/models/models.go`。
2. 在 `repository.migrate` 中添加幂等迁移。
3. 更新 `scanShare`、写入 SQL、列表 SQL。
4. 更新 `service.toSummary` 和必要的 JSON 字段。
5. 更新模板和前端 JS。
6. 增加 repository 或 service 测试。

调整过期或删除规则：

1. 过期访问规则位于 `models.Share`；实际文件删除仅由 `service.DeleteShare` 执行，不能在会话清理任务中删除分享文件。
2. 保持软删除语义：数据库记录保留，`deleted_at` 表示删除状态。
3. 文件删除和数据库状态无法跨系统原子提交：先检查文件删除结果，再标记删除；失败时保留记录供重试，已不存在的文件不应阻止重试。
4. 补充 `internal/service/service_test.go` 中对应场景。

调整上传能力：

1. 表单入口在 `web/templates/upload.html` 和 `web/static/app.js`。
2. 服务端入口在 `handleCreateFileShare`。
3. 文件保存逻辑在 `service.CreateFileShare` 和 `saveUploadedFile`。
4. 存储命名格式当前为 `<shareID>-<fileID><原扩展名>`。
5. 如需增加文件类型限制、单文件大小限制、总大小限制，应在服务端强制校验，前端只做辅助提示。

## 代码风格和维护建议

- 保持现有 Go 风格，优先使用 `gofmt`。
- Handler 只做 HTTP 解析、状态码和渲染，业务判断放到 service。
- Repository 只负责 SQL 和数据转换，不放业务规则。
- Service 是业务边界，适合放测试。
- 对外展示的错误信息使用中文，内部包装错误可保留英文上下文。
- 文件系统操作失败时要考虑回滚或清理，参考 `CreateFileShare` 中的 `createdPaths`。
- 时间写入数据库前统一用 UTC，展示时使用 `Local()`。

## AI 修改前检查清单

- 是否先读了相关源码，而不只依赖本文档？
- 是否需要更新 `.env.example` 或 README？
- 是否涉及数据库迁移，迁移是否幂等？
- 是否涉及安全敏感数据，是否避免明文保存或日志输出？
- 是否改变了公开路由、表单字段或 JSON 字段，前后端是否同步？
- 是否运行了 `go test ./...`？
- 是否避免改动 `data/`、上传文件和本地 `.env`？
