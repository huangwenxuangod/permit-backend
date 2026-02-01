# Permit Backend

简洁易用的证件照与订单服务后端，采用四层架构（server/usecase/domain/infrastructure），内置算法服务接入与微信支付参数（mock），可在内存与 PostgreSQL 持久化之间切换。

## 特性
- 分层清晰：接口适配层（Gin）、应用用例层、领域模型层、基础设施适配器层
- 算法集成：上传图片生成证件照（背景色批量处理），兼容 data URL base64
- 支付集成：返回微信 JSAPI v3 风格参数（mock，便于前端联调）
- 存储可切换：默认内存；设置 POSTGRES_DSN 即启用 PostgreSQL 持久化
- 跨端易用：Windows/PowerShell 与 curl 均可快速调用

## 依赖与环境
- Go >= 1.25
- Gin 框架（已在 go.mod 中声明）
- PostgreSQL（可选）
- 本地算法服务（默认 http://127.0.0.1:8080，可配置）

### 环境变量
- PERMIT_ENV、PERMIT_PORT、PERMIT_ASSETS_DIR、PERMIT_UPLOADS_DIR
- PERMIT_JWT_SECRET、PERMIT_LOG_JSON、PERMIT_ALGO_URL
- PERMIT_PAY_MOCK、PERMIT_WECHAT_APPID、PERMIT_WECHAT_MCHID、PERMIT_WECHAT_NOTIFY_URL
- POSTGRES_DSN

示例（.env.local 或系统环境）:

```
PERMIT_ENV=dev
PERMIT_PORT=5000
PERMIT_ASSETS_DIR=./assets
PERMIT_UPLOADS_DIR=./uploads
PERMIT_ALGO_URL=http://127.0.0.1:8080
PERMIT_PAY_MOCK=true
POSTGRES_DSN=postgres://postgres:yourpass@127.0.0.1:5432/permit?sslmode=disable
```

## 快速开始

### 拉取依赖与构建

```
go mod tidy
go build ./...
```

### 运行服务

```
go run ./cmd/permit-backend
```

启动后输出当前配置；若 POSTGRES_DSN 非空则自动启用 PostgreSQL 并建表（开发模式）。

## API 速览

- 上传文件：POST /api/upload（form-data: file）→ 返回 objectKey
- 创建任务：POST /api/tasks → 生成证件照（按颜色输出多张）
- 查询任务：GET /api/tasks/{id}
- 下载信息：GET /api/download/{id}（任务完成后返回 URLs）
- 创建订单：POST /api/orders
- 查询订单：GET /api/orders、GET /api/orders/{id}
- 支付参数（mock）：POST /api/pay/wechat
- 回调更新：POST /api/pay/callback

### PowerShell 示例（Windows）

```
$env:POSTGRES_DSN = 'postgres://postgres:yourpass@127.0.0.1:5432/permit?sslmode=disable'
$env:PERMIT_ALGO_URL = 'http://127.0.0.1:8080'
go run .\cmd\permit-backend

$up = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/upload -Form @{ file=Get-Item 'D:\path\to\test.jpg' }
$body = @{ specCode='passport'; sourceObjectKey=$up.objectKey; colors=@('white','blue'); widthPx=354; heightPx=472; dpi=300 } | ConvertTo-Json
$task = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/tasks -ContentType 'application/json' -Body $body

$orderBody = @{ taskId=$task.id; items=@(@{type='print';qty=1}); city='上海'; remark='测试'; amountCents=990; channel='wechat' } | ConvertTo-Json
$order = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/orders -ContentType 'application/json' -Body $orderBody
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/pay/wechat -ContentType 'application/json' -Body (@{orderId=$order.orderId} | ConvertTo-Json)
```

### curl 示例

```
curl -F file=@./test.jpg http://127.0.0.1:5000/api/upload
curl -H "Content-Type: application/json" -d '{"specCode":"passport","sourceObjectKey":"uploads/xxx.jpg","colors":["white","blue"],"widthPx":354,"heightPx":472,"dpi":300}' http://127.0.0.1:5000/api/tasks
```

## 目录结构

```
cmd/permit-backend/main.go         # 启动入口
internal/
  server/server.go                 # 接口适配（Gin 路由）
  usecase/
    task_service.go                # 任务用例
    order_service.go               # 订单用例
  domain/
    task.go                        # 任务模型
    order.go                       # 订单模型
  infrastructure/
    repo/memory.go                 # 内存仓库实现
    repo/postgres.go               # Postgres 仓库实现
    asset/writer.go                # 资产写入（FS）
  algo/client.go                   # 算法客户端
  config/config.go                 # 配置结构与环境变量映射
  env/env.go                       # .env 加载器
docs/
  开发架构规范.md                  # 开发架构规范（分层、约定、流程）
```

## 开发注意
- 修改依赖后运行 `go mod tidy`
- 不提交敏感信息到仓库（秘钥经环境变量传入）
- 生产环境建议引入数据库迁移工具，替代运行时建表

## 参考文档
- 开发架构规范： [docs/开发架构规范.md](docs/开发架构规范.md)
