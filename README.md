# BigFile WebDAV Gateway

Go WebDAV 网关与 React 上传界面。WebDAV 数据面保持只读；上传界面按 BigFile 协议上传文件/文件夹并生成新的分享 hash。

## 功能

- `PROPFIND`：由本服务返回 WebDAV 元数据。
- `GET` / `HEAD`：文件返回 `307 Temporary Redirect` 到 BigFile 直链。
- WebDAV 写方法仍返回 `405`，不支持通过 WebDAV 修改文件。
- `/ui/`：React + TypeScript + shadcn 风格上传界面。
- `/api/upload`：同源上传代理，仅接受 BigFile 的原始字节上传协议。
- 支持文件、多个文件、文件夹结构、SHA-256/Base62、96 MiB 分片和逐文件进度。
- 上传全部文件后自动上传 `list.json`，输出分享链接及 `BIGFILE_SHARE_HASH`。

## 架构

浏览器不能直接跨域调用 BigFile 上传节点，前端将原始分片发到同源 `/api/upload`。Go 服务校验 `Content-Range`、`Hash`、`Next` 后流式转发到固定上传节点，不做 multipart 包装，也不临时落盘。

文件下载仍使用 `307` 重定向，下载字节不经过本服务。上传字节会经过代理，因此公网部署应在反向代理处增加认证、限流和 TLS。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `BIGFILE_SHARE_HASH` | 必填 | 当前 WebDAV 挂载的 BigFile 分享 hash（Base62） |
| `LISTEN_ADDR` | `:8080` | HTTP 监听地址 |
| `DAV_PREFIX` | `/` | WebDAV 挂载前缀，推荐设为 `/dav` |
| `BIGFILE_BASE_URL` | `https://www.bigfile.net` | 分享清单与下载站点 |
| `BIGFILE_UPLOAD_URL` | `https://u1.bigfile.net/v1/upload` | 固定上传节点 |
| `REFRESH_INTERVAL` | `5m` | 清单刷新周期，`0` 表示关闭 |
| `HTTP_TIMEOUT` | `15s` | 获取 `list.json` 的超时 |
| `UPLOAD_TIMEOUT` | `30m` | 单个上传请求的上游超时 |
| `WEB_DIR` | `web/dist` | React 构建产物目录 |

## 本地开发

后端：

```bash
export BIGFILE_SHARE_HASH=Ab12Cd
export DAV_PREFIX=/dav
go run ./cmd/bigfile-webdav
```

前端：

```bash
cd web
npm install
npm run dev
```

打开 `http://127.0.0.1:5173/`。Vite 会将 `/api/upload` 直接代理到 BigFile 上传节点，因此可以先用开发模式生成首个分享 hash，再将结果配置为 `BIGFILE_SHARE_HASH`。

生产构建后，Go 服务也会在 `http://127.0.0.1:8080/ui/` 提供前端：

```bash
cd web && npm ci && npm run build
cd ..
go run ./cmd/bigfile-webdav
```

## Docker

```bash
docker build -t bigfile-webdav .
docker run --rm -p 8080:8080 \
  -e BIGFILE_SHARE_HASH=Ab12Cd \
  -e DAV_PREFIX=/dav \
  bigfile-webdav
```

访问：

- 上传界面：`http://127.0.0.1:8080/ui/`
- WebDAV：`http://127.0.0.1:8080/dav/`
- 上传代理：`POST http://127.0.0.1:8080/api/upload`

## WebDAV 示例

```bash
curl -X PROPFIND -H 'Depth: 1' http://127.0.0.1:8080/dav/
curl -i http://127.0.0.1:8080/dav/docs/report.txt
```

文件请求会得到 `307`，客户端应跟随跨主机重定向访问 BigFile。若要直接下载可使用 `curl -L`。

`rclone` 配置：

```ini
[bigfile]
type = webdav
url = http://127.0.0.1:8080/dav/
vendor = other
```

## 安全边界

- WebDAV 不实现认证，公网部署必须放在受保护的 HTTPS 反向代理之后。
- `/api/upload` 只能代理到配置的 `BIGFILE_UPLOAD_URL`，不会接受客户端指定的目标地址。
- 每个上传请求最多 96 MiB；大文件由浏览器分片。
- 代理只转发 `Content-Range`、`Hash`、`Next` 和 `Content-Type`。
- 分享 hash 更新后需要用新的 `BIGFILE_SHARE_HASH` 重启服务，WebDAV 才会挂载新清单。
