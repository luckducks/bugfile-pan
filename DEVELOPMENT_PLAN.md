# BigFile WebDAV 网关开发计划

## 1. 目标与边界

实现一个 **只读 WebDAV 网关**，把 BigFile 分享清单映射成 WebDAV 目录树：

- `PROPFIND` 等元数据请求由本服务响应。
- 文件 `GET` / `HEAD` 返回到 `https://www.bigfile.net/d/{hash}/{filename}?content-type={mime}` 的临时重定向，文件字节不经过本服务。
- BigFile 的 `list.json` 很小，仅作为控制面元数据由服务获取；文件数据面不代理、不缓存。
- 第一版明确不支持 `PUT`、`DELETE`、`MKCOL`、`MOVE`、`COPY`、`LOCK`、`UNLOCK`、`PROPPATCH`。

## 2. 为什么第一版必须只读

`BIGFILE_API_DOC.md` 只提供内容寻址上传、下载和分享清单，没有按路径列目录、删除、重命名等接口。WebDAV `PUT` 的请求体是文件本身，而 BigFile 上传要求在上传前知道完整内容的 SHA-256/Base62，并设置 `Hash`、`Content-Range`；因此不能在“不让文件流量经过本服务”的前提下把任意 WebDAV `PUT` 直接重定向为 BigFile 上传。跨主机重定向的客户端兼容性也不稳定。

如果以后需要可写版本，只能另立方案：客户端直传扩展/预签名流程，或者允许服务端暂存并转传（后者违反当前数据面不经过服务的要求）。

## 3. 技术设计

### 3.1 进程配置

环境变量：

- `BIGFILE_SHARE_HASH`：必填，挂载的分享 hash。
- `LISTEN_ADDR`：默认 `:8080`。
- `DAV_PREFIX`：默认 `/`，可设为 `/dav`。
- `BIGFILE_BASE_URL`：默认 `https://www.bigfile.net`，便于测试及私有兼容端点。
- `REFRESH_INTERVAL`：默认 `5m`；设为 `0` 禁用定时刷新。
- `HTTP_TIMEOUT`：默认 `15s`，仅用于获取清单。

### 3.2 模块

- `internal/bigfile/client.go`
  - 获取 `/d/{shareHash}/list.json`。
  - 限制响应大小、校验 HTTP 状态和 JSON 字段。
  - 生成安全的文件直链。
- `internal/catalog/catalog.go`
  - 将 `path + name` 构造成不可变目录树快照。
  - 拒绝绝对路径、`.`/`..` 越界、空文件名、带 `/` 的文件名、重复路径及文件/目录冲突。
  - 原子替换快照；刷新失败保留上一份可用快照。
- `internal/dav/handler.go`
  - `OPTIONS` 宣告只读能力。
  - 文件 `GET` / `HEAD` 返回 `307 Temporary Redirect`，不读取上游文件内容。
  - `PROPFIND` 交给 `golang.org/x/net/webdav.Handler`，其底层使用只读虚拟 `FileSystem`。
  - 目录 `GET` 返回 405；所有写方法返回 405，并附 `Allow`。
- `cmd/bigfile-webdav/main.go`
  - 加载配置、首次同步（失败即退出）、启动周期刷新、HTTP 服务和优雅退出。

### 3.3 WebDAV 文件系统

使用 `golang.org/x/net/webdav` 的 `FileSystem` 接口承载只读目录树。官方实现中 `Handler` 会把 `PROPFIND` 分派给属性处理器，而标准 `GET` 会打开文件后调用 `http.ServeContent`，会导致服务端读取文件，所以本项目必须在外层拦截文件 `GET`/`HEAD` 并直接重定向。

依据：

- Handler 方法分派：<https://github.com/golang/net/blob/a3c1227e666da136a7d1dbc685c4842492d34c36/webdav/webdav.go#L55-L91>
- 标准 GET 会执行 `OpenFile` 和 `http.ServeContent`：<https://github.com/golang/net/blob/a3c1227e666da136a7d1dbc685c4842492d34c36/webdav/webdav.go#L209-L235>
- FileSystem/File 接口：<https://github.com/golang/net/blob/a3c1227e666da136a7d1dbc685c4842492d34c36/webdav/file.go#L39-L62>
- 自定义 `ETager` 可把 BigFile hash 暴露为稳定 ETag：<https://github.com/golang/net/blob/a3c1227e666da136a7d1dbc685c4842492d34c36/webdav/prop.go#L433-L463>

## 4. HTTP 行为

| 请求 | 文件 | 目录/根 | 不存在 |
|---|---:|---:|---:|
| `OPTIONS` | 200 | 200 | 200 |
| `PROPFIND` | 207 | 207 | 404 |
| `GET` | 307 到 BigFile | 405 | 404 |
| `HEAD` | 307 到 BigFile | 405 | 404 |
| 写方法 | 405 | 405 | 405 |

重定向 URL 的 hash 和文件名按 URL path segment 编码，MIME 使用 query 编码。保留客户端原请求（包括 `Range`）由客户端在跟随重定向后重新发送；服务本身不发起文件下载请求。

## 5. 安全与可靠性

- 限制 `list.json` 最大体积和条目数，防止内存耗尽。
- 对 share hash 做 Base62 字符校验，避免构造任意上游路径。
- 清理和验证清单路径，阻止路径穿越及树冲突。
- 上游客户端设置超时；只接受成功状态。
- 周期刷新构建完整新快照后一次性替换，读请求始终看到一致目录树。
- HTTP Server 配置 header/read/idle 超时；日志不输出潜在敏感 header。

## 6. 测试与验收

单元/集成测试至少覆盖：

1. 正常清单生成嵌套目录及属性。
2. 根、目录、文件的 `PROPFIND`（Depth 0/1）。
3. 文件 `GET` 和 `HEAD` 返回正确 307 Location。
4. 下载上游端点未被本服务访问，证明数据不经服务中转。
5. 写方法统一返回 405。
6. 前缀挂载、URL 编码、MIME query 编码。
7. 恶意/非法路径、重复项、目录文件冲突被拒绝。
8. 清单超限、非 2xx、非法 JSON 被拒绝。
9. 刷新失败时旧快照仍可服务。
10. 执行 `gofmt`、`go vet ./...`、`go test ./...`，必要时运行 `go test -race ./...`。

## 7. 交付物

- 可运行 Go module 和命令行入口。
- 单元及 HTTP 集成测试。
- `README.md`（配置、运行、curl/rclone 示例、只读限制和重定向兼容性说明）。
- `Dockerfile` 与 `.dockerignore`。
