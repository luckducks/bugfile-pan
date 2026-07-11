# BigFile.net 非官方 API 文档

> 来源：基于 `https://www.bigfile.net/` 前端代码与 Chrome 实际抓包整理  
> 说明：这是逆向分析结果，不代表官方稳定协议

---

# 1. 基础信息

## 1.1 Host 说明

BigFile 使用两类域名：

| 类型 | Host 示例 | 用途 |
|---|---|---|
| 主站 | `https://www.bigfile.net` | 页面展示、分享页、文件下载、list.json |
| 上传节点 | `https://u1.bigfile.net` | 文件上传 |
| 上传节点 | `https://u2.bigfile.net` | 文件上传 |

前端会创建多个上传 iframe，例如：

- `https://u1.bigfile.net/upload`
- `https://u2.bigfile.net/upload`

实际上传请求发往：

- `POST https://u1.bigfile.net/v1/upload`
- `POST https://u2.bigfile.net/v1/upload`

---

## 1.2 通用特征

| 项目 | 说明 |
|---|---|
| 鉴权 | 无鉴权，无登录态 |
| 上传 body | 原始字节流，不是 `multipart/form-data` |
| 文件标识 | SHA-256 结果转 Base62 |
| 大文件上传 | 分片上传，依赖 `Next` header |
| 下载接口 | 通过 `/d/{hash}/{filename}` 访问 |
| 分享功能 | 本质是上传一个 `list.json` 文件 |

---

# 2. 数据结构定义

---

## 2.1 File Hash

前端会先计算文件完整内容的 SHA-256，然后转成 Base62 字符串。

| 字段 | 类型 | 说明 |
|---|---|---|
| `hash` | `string` | 文件内容 SHA-256 转 Base62 后的值 |

### 示例
```text
Aeo7HbT3j4TvaA0SlueQYMNLP6S43jNjrIbYLeK5ySK
r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4
```

---

## 2.2 分享列表文件对象

分享页 `list.json` 中每个文件对象结构如下：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | `string` | 是 | 文件名 |
| `size` | `number` | 是 | 文件大小，单位字节 |
| `type` | `string` | 是 | MIME 类型 |
| `hash` | `string` | 是 | 文件 hash（Base62） |
| `path` | `string` | 是 | 相对目录路径，如 `./` |

### 示例
```json
{
  "name": "binary.bin",
  "size": 20,
  "type": "application/octet-stream",
  "hash": "r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4",
  "path": "./"
}
```

---

# 3. 接口文档

---

## 3.1 上传文件

### 接口
`POST /v1/upload`

### Host
- `https://u1.bigfile.net`
- `https://u2.bigfile.net`

### 用途
上传文件或上传文件分片。

---

### 请求头

| Header | 类型 | 必填 | 说明 |
|---|---|---|---|
| `Content-Range` | `string` | 是 | 当前上传字节范围，格式为 `{start}-{end}` |
| `Hash` | `string` | 条件必填 | 完整文件 SHA-256 转 Base62；单片上传必填，分片上传最后一片必填 |
| `Next` | `string` | 条件必填 | 分片续传 token；第二片及后续分片需要 |

---

### 请求体

| 类型 | 说明 |
|---|---|
| `application/octet-stream` 风格原始 body | 直接发送文件字节，不带 multipart 包装 |

> 实际前端调用方式为：`xhr.send(blob)`

---

### 响应头

| Header | 示例 | 说明 |
|---|---|---|
| `content-type` | `application/json` | JSON 响应 |

---

### 响应体结构

#### 单片上传完成 / 分片最终完成
| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | `boolean` | 是否成功 |
| `uri` | `string` | 文件资源基础路径 |

#### 分片中间响应
| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | `boolean` | 是否成功 |
| `next` | `string` | 下一个分片所需 token |

---

### 成功响应示例：文本文件
```json
{
  "status": true,
  "uri": "/d/Aeo7HbT3j4TvaA0SlueQYMNLP6S43jNjrIbYLeK5ySK"
}
```

### 成功响应示例：二进制文件
```json
{
  "status": true,
  "uri": "/d/r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4"
}
```

### 分片首片响应示例
```json
{
  "status": true,
  "next": "3ed77f173d24bbb4173acda73268b862201ffbe0ff6435e5c0d712f37b6ee6664f3fb714dfa0c2cf5b4861dc2abbf52210b85c66b0c411c40d2615668271c5b2cb69b818d6cd2c8739c18a0ad50e6addd831c0a029e8d9ccf9831f505dc0b09cd569e2ac4638c2bc1f6a584f98b5bfb7eabbbb76dedea4ba7c2300c2e098a47dc65e68e554bec64a97c6440b707bc62161a8fef1a922040e1029e2a6605f850b2a20aa1141f0257726bef81bf28b761f86e17b54669f3ffe955bbf381b8e62c7626448629e6c8066a8d32124e43c1bf2125962d2589df7e24ff1e9d16496c61f2d04ab5c40a8e0f6c354c88199b5d913f0145312a5869d7ceb0bf0b801341f74c52698529c52af7954337d55a7f60ff69a3563f12507a6db3026d6f60b40b486127edeb826dc8ad84e3a4eda0fe0a185c00df01e391fece555e75c527139297030ead7bedc1ed8d7321d7e1defc2895ffce484c78a95730fa90aee9ca86d213105b5cfbb254022f7ce0d476b3d4cff3164f80dbdee5efa6de0d7c551b0aa2bbc6c243c9cccf83cc9970eb4cbb2a6ef091b3a552f0e5472a1b35ac8b3f46341feb202e9d955b7641d9de41da81d0be340dcd82e01ab6935d23dc03cb9c7f83d8a69da99328124960f7369bf4ef41e20a7c6dbf627d76646a700871e24ddff3e9a1b475e3b836d6cf500c3f036"
}
```

---

### 示例请求 1：上传文本文件

#### 请求
```http
POST /v1/upload HTTP/1.1
Host: u1.bigfile.net
Content-Range: 0-4
Hash: Aeo7HbT3j4TvaA0SlueQYMNLP6S43jNjrIbYLeK5ySK

hello
```

#### 请求体说明
文件内容：
```txt
hello
```

---

### 示例请求 2：上传二进制文件

#### 请求
```http
POST /v1/upload HTTP/1.1
Host: u1.bigfile.net
Content-Range: 0-19
Hash: r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4

<20 bytes raw binary>
```

#### 二进制请求体字节
```text
[0,1,2,3,255,254,128,64,10,13,0,99,100,101,250,251,252,253,254,255]
```

#### 响应
```json
{
  "status": true,
  "uri": "/d/r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4"
}
```

---

## 3.2 分片上传：第一片

### 接口
`POST /v1/upload`

### 用途
上传大文件第一片，服务端返回 `next` token。

---

### 请求头

| Header | 示例 | 必填 | 说明 |
|---|---|---|---|
| `Content-Range` | `0-2` | 是 | 当前片范围 |
| `Hash` | 无 | 否 | 首片通常不带 |
| `Next` | 无 | 否 | 首片不带 |

---

### 请求体
当前分片原始字节。

---

### 响应体
```json
{
  "status": true,
  "next": "..."
}
```

---

### 示例请求
```http
POST /v1/upload HTTP/1.1
Host: u1.bigfile.net
Content-Range: 0-2

HEL
```

### 示例响应
```json
{
  "status": true,
  "next": "3ed77f173d24bbb4173acda73268b862201ffbe0ff6435e5c0d712f37b6ee6664f3fb714dfa0c2cf5b4861dc2abbf52210b85c66b0c411c40d2615668271c5b2cb69b818d6cd2c8739c18a0ad50e6addd831c0a029e8d9ccf9831f505dc0b09cd569e2ac4638c2bc1f6a584f98b5bfb7eabbbb76dedea4ba7c2300c2e098a47dc65e68e554bec64a97c6440b707bc62161a8fef1a922040e1029e2a6605f850b2a20aa1141f0257726bef81bf28b761f86e17b54669f3ffe955bbf381b8e62c7626448629e6c8066a8d32124e43c1bf2125962d2589df7e24ff1e9d16496c61f2d04ab5c40a8e0f6c354c88199b5d913f0145312a5869d7ceb0bf0b801341f74c52698529c52af7954337d55a7f60ff69a3563f12507a6db3026d6f60b40b486127edeb826dc8ad84e3a4eda0fe0a185c00df01e391fece555e75c527139297030ead7bedc1ed8d7321d7e1defc2895ffce484c78a95730fa90aee9ca86d213105b5cfbb254022f7ce0d476b3d4cff3164f80dbdee5efa6de0d7c551b0aa2bbc6c243c9cccf83cc9970eb4cbb2a6ef091b3a552f0e5472a1b35ac8b3f46341feb202e9d955b7641d9de41da81d0be340dcd82e01ab6935d23dc03cb9c7f83d8a69da99328124960f7369bf4ef41e20a7c6dbf627d76646a700871e24ddff3e9a1b475e3b836d6cf500c3f036"
}
```

---

## 3.3 分片上传：后续片 / 最后一片

### 接口
`POST /v1/upload`

### 用途
继续上传后续分片，最后一片提交完整文件 `Hash`。

---

### 请求头

| Header | 类型 | 必填 | 说明 |
|---|---|---|---|
| `Content-Range` | `string` | 是 | 当前片范围 |
| `Next` | `string` | 是 | 来自上一片响应 |
| `Hash` | `string` | 最后一片必填 | 完整文件 SHA-256 的 Base62 |

---

### 请求体
当前片原始字节。

---

### 成功响应
```json
{
  "status": true,
  "uri": "/d/D5ae7vcGrecE8vye6USG2nBMe3PfT4DQRHE8l2XwUJ3"
}
```

---

### 示例请求
```http
POST /v1/upload HTTP/1.1
Host: u1.bigfile.net
Content-Range: 3-4
Next: 3ed77f173d24bbb4173acda73268b86220...
Hash: D5ae7vcGrecE8vye6USG2nBMe3PfT4DQRHE8l2XwUJ3

LO
```

---

## 3.4 获取文件内容（预览/直读）

### 接口
`GET /d/{hash}/{filename}?content-type={mime}`

### Host
通常为：
- `https://www.bigfile.net`
- 也可在上传域下读取，例如 `https://u1.bigfile.net`

### 用途
获取文件原始内容，并按 `content-type` 指定响应 MIME。

---

### 路径参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `hash` | `string` | 是 | 文件 hash（Base62） |
| `filename` | `string` | 是 | 文件名 |

---

### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `content-type` | `string` | 是 | 希望服务端返回的 MIME 类型 |

---

### 响应头

| Header | 说明 |
|---|---|
| `content-type` | 按 query 中的 `content-type` 返回 |
| `content-length` | 文件大小 |
| `accept-ranges` | 支持字节范围 |
| `etag` | 文件 ETag |
| `last-modified` | 文件修改时间 |

---

### 示例请求：文本文件
```http
GET /d/Aeo7HbT3j4TvaA0SlueQYMNLP6S43jNjrIbYLeK5ySK/hello.txt?content-type=text%2Fplain HTTP/1.1
Host: www.bigfile.net
```

### 示例响应头
```http
HTTP/1.1 200 OK
content-length: 5
content-type: text/plain
accept-ranges: bytes
etag: "5d41402abc4b2a76b9719d911017c592"
```

### 示例响应体
```txt
hello
```

---

### 示例请求：二进制文件
```http
GET /d/r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4/binary.bin?content-type=application%2Foctet-stream HTTP/1.1
Host: www.bigfile.net
```

### 示例响应头
```http
HTTP/1.1 200 OK
content-length: 20
content-type: application/octet-stream
accept-ranges: bytes
etag: "7a9d3889ee04c7ad841286dff264dc33"
```

### 示例响应体字节
```text
[0,1,2,3,255,254,128,64,10,13,0,99,100,101,250,251,252,253,254,255]
```

---

## 3.5 下载文件

### 接口
`GET /d/{hash}/{filename}?content-type={mime}&download`

### 用途
强制下载文件。

---

### 路径参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `hash` | `string` | 是 | 文件 hash（Base62） |
| `filename` | `string` | 是 | 文件名 |

---

### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `content-type` | `string` | 是 | MIME 类型 |
| `download` | 无值参数 | 是 | 存在即触发附件下载 |

---

### 响应头

| Header | 说明 |
|---|---|
| `content-disposition` | 指定附件下载文件名 |
| `content-type` | 常见为 `application/octet-stream` |
| `content-length` | 文件大小 |

---

### 示例请求
```http
GET /d/r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4/binary.bin?content-type=application%2Foctet-stream&download HTTP/1.1
Host: www.bigfile.net
```

### 示例响应头
```http
HTTP/1.1 200 OK
content-disposition: attachment; filename*=UTF-8''binary.bin
content-type: application/octet-stream
content-length: 20
```

### 示例响应体
原始文件字节流。

---

## 3.6 获取分享列表

### 接口
`GET /d/{shareHash}/list.json`

### 用途
获取分享页面对应的文件列表。

---

### 路径参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `shareHash` | `string` | 是 | 分享 JSON 文件的 hash（Base62） |

---

### 请求体
无

---

### 响应头

| Header | 说明 |
|---|---|
| `content-type` | `application/json` |

---

### 响应体

| 类型 | 说明 |
|---|---|
| `Array<ShareFileItem>` | 分享中的文件列表 |

---

### 示例请求
```http
GET /d/YUgUv3ZOeIfp52gOAQlTbrEZ9qX8oSx2FmLOQovyDET/list.json HTTP/1.1
Host: www.bigfile.net
```

### 示例响应
```json
[
  {
    "name": "binary.bin",
    "size": 20,
    "type": "application/octet-stream",
    "hash": "r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4",
    "path": "./"
  }
]
```

---

## 3.7 分享页面入口

### 接口
`GET /s/{shareHash}`

### 用途
打开分享页。

---

### 路径参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `shareHash` | `string` | 是 | 对应分享 JSON 文件 hash |

---

### 说明
这个地址本身不是 JSON API，它是前端分享页路由。

页面打开后，前端会再请求：

```http
GET /d/{shareHash}/list.json
```

---

### 示例请求
```http
GET /s/YUgUv3ZOeIfp52gOAQlTbrEZ9qX8oSx2FmLOQovyDET HTTP/1.1
Host: www.bigfile.net
```

---

# 4. 分享创建机制

分享不是独立的“创建分享”接口，而是复用了上传接口。

---

## 4.1 分享创建流程

| 步骤 | 说明 |
|---|---|
| 1 | 普通文件先各自上传，得到各自 hash |
| 2 | 前端组装分享 JSON 数组 |
| 3 | 把这个 JSON 作为一个文件上传到 `/v1/upload` |
| 4 | 上传返回 `/d/{shareHash}` |
| 5 | 前端构造分享链接 `/s/{shareHash}` |

---

## 4.2 分享 JSON 示例

```json
[
  {
    "name": "binary.bin",
    "size": 20,
    "type": "application/octet-stream",
    "hash": "r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4",
    "path": "./"
  }
]
```

---

## 4.3 上传分享 JSON 的请求示例

```http
POST /v1/upload HTTP/1.1
Host: u1.bigfile.net
Content-Range: 0-177
Hash: YUgUv3ZOeIfp52gOAQlTbrEZ9qX8oSx2FmLOQovyDET

[
  {
    "name": "binary.bin",
    "size": 20,
    "type": "application/octet-stream",
    "hash": "r53JUNUD3XPIvMn0A6RoVNHOBYXI1632TUW2MfhLwo4",
    "path": "./"
  }
]
```

### 响应
```json
{
  "status": true,
  "uri": "/d/YUgUv3ZOeIfp52gOAQlTbrEZ9qX8oSx2FmLOQovyDET"
}
```

### 对应分享页
```text
https://www.bigfile.net/s/YUgUv3ZOeIfp52gOAQlTbrEZ9qX8oSx2FmLOQovyDET
```

---

# 5. 错误与异常说明

前端代码显示，上传响应如果包含：

```json
{
  "status": false,
  "message": "..."
}
```

则会被视为失败。

---

## 5.1 可能的错误响应结构

| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | `boolean` | 成功/失败 |
| `message` | `string` | 错误信息 |

### 示例
```json
{
  "status": false,
  "message": "server error"
}
```

---

# 6. 关键实现约束

| 项目 | 约束 |
|---|---|
| 小文件阈值 | `<= 96MB` 单片上传 |
| 大文件阈值 | `> 96MB` 分片上传 |
| 分片大小 | `96MB` |
| hash 算法 | SHA-256 |
| hash 编码 | Base62 |
| 上传方式 | XHR + 原始 Blob |
| 下载缓存 | CloudFront/CDN 可缓存 |

---

# 7. 最小调用模板

---

## 7.1 单文件上传模板

```http
POST /v1/upload
Content-Range: 0-{size-1}
Hash: {base62_sha256}

<raw bytes>
```

成功返回：
```json
{
  "status": true,
  "uri": "/d/{hash}"
}
```

---

## 7.2 分片上传模板

### 第一片
```http
POST /v1/upload
Content-Range: {start}-{end}

<chunk bytes>
```

响应：
```json
{
  "status": true,
  "next": "{token}"
}
```

### 后续片
```http
POST /v1/upload
Content-Range: {start}-{end}
Next: {token}
Hash: {base62_sha256_if_final}

<chunk bytes>
```

最终响应：
```json
{
  "status": true,
  "uri": "/d/{hash}"
}
```

---

## 7.3 下载模板

### 预览
```http
GET /d/{hash}/{filename}?content-type={mime}
```

### 下载
```http
GET /d/{hash}/{filename}?content-type={mime}&download
```
