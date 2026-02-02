# PostgreSQL 启用与端到端验证（Windows/PowerShell）

## 前提
- 已在本机安装 PostgreSQL（默认端口 5432）
- 拥有数据库超级用户或具备创建库的权限（示例使用 postgres 用户）
- 已安装 Go（>= 1.25）

## 步骤一：创建数据库
在 PowerShell 中创建数据库 permit（如已存在可跳过）：

```powershell
psql -U postgres -h 127.0.0.1 -c "CREATE DATABASE permit;"
```

如果提示 psql 不在 PATH，可使用 PostgreSQL 安装目录中的 psql.exe 或使用图形工具（PgAdmin）创建。

## 步骤二：写入环境变量（.env.local）
将连接字符串写入项目根目录的 .env.local（若文件不存在会自动创建）：

```powershell
Set-Location d:\dev\one-permit\permit-backend
"POSTGRES_DSN=postgres://postgres:yourpass@127.0.0.1:5432/permit?sslmode=disable" | Out-File -FilePath .env.local -Append -Encoding utf8
"PERMIT_ALGO_URL=http://127.0.0.1:8080" | Out-File -FilePath .env.local -Append -Encoding utf8
"PERMIT_PAY_MOCK=true" | Out-File -FilePath .env.local -Append -Encoding utf8
```

可选：也可以在当前会话直接设置环境变量（仅本次会话生效）：

```powershell
$env:POSTGRES_DSN = 'postgres://postgres:yourpass@127.0.0.1:5432/permit?sslmode=disable'
$env:PERMIT_ALGO_URL = 'http://127.0.0.1:8080'
$env:PERMIT_PAY_MOCK = 'true'
```

## 步骤三：拉取依赖与编译

```powershell
go mod tidy
go build ./...
```

## 步骤四：启动服务

```powershell
go run .\cmd\permit-backend
```

启动时会打印当前配置（JSON）。确认其中 `PostgresDSN` 非空，表示已启用 PostgreSQL 仓库。参考代码：
- 配置加载：[config.go](file:///d:/dev/one-permit/permit-backend/internal/config/config.go)
- 启动入口：[main.go](file:///d:/dev/one-permit/permit-backend/cmd/permit-backend/main.go)
- 仓库选择逻辑：[server.go](file:///d:/dev/one-permit/permit-backend/internal/server/server.go)

## Docker 一键启动（可选）
如需一次性启动后端 + PostgreSQL + 算法服务，可使用 docker compose：

```powershell
docker compose up -d
docker compose ps
```

默认端口：
- 后端：5000
- 算法服务：8080
- PostgreSQL：5432

## 步骤五：上传与创建任务

```powershell
# 上传图片，得到 objectKey
$up = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/upload -Form @{ file=Get-Item 'D:\dev\one-permit\HivisionIDPhotos\demo\images\test0.jpg' }
$up

# 创建任务（使用上传返回的 objectKey）
$taskBody = @{
  specCode='passport'
  sourceObjectKey=$up.objectKey
  colors=@('white','blue')
  widthPx=354
  heightPx=472
  dpi=300
} | ConvertTo-Json

$task = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/tasks -ContentType 'application/json' -Body $taskBody
$task

# 查询任务
Invoke-RestMethod -Method Get -Uri ("http://127.0.0.1:5000/api/tasks/" + $task.id)
```

提示：算法服务未运行时任务可能为 `failed` 状态，依然会写入数据库，可用于验证持久化。

## 步骤六：创建订单、支付参数与回调

```powershell
# 创建订单
$orderBody = @{
  taskId=$task.id
  items=@(@{type='print';qty=1})
  city='上海'
  remark='测试'
  amountCents=990
  channel='wechat'
} | ConvertTo-Json

$order = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/orders -ContentType 'application/json' -Body $orderBody
$order

# 查询订单
Invoke-RestMethod -Method Get -Uri ("http://127.0.0.1:5000/api/orders/" + $order.orderId)

# 获取微信支付参数（mock）
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/pay/wechat -ContentType 'application/json' -Body (@{orderId=$order.orderId} | ConvertTo-Json)

# 模拟支付回调
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5000/api/pay/callback -ContentType 'application/json' -Body (@{orderId=$order.orderId;status='paid'} | ConvertTo-Json)
```

## 步骤七：数据库侧验证

```powershell
psql -U postgres -h 127.0.0.1 -d permit -c "SELECT COUNT(1) FROM tasks;"
psql -U postgres -h 127.0.0.1 -d permit -c "SELECT COUNT(1) FROM orders;"
psql -U postgres -h 127.0.0.1 -d permit -c "SELECT order_id,task_id,amount_cents,channel,status,created_at FROM orders ORDER BY created_at DESC LIMIT 5;"
```

若能查询到任务与订单记录，说明持久化已生效。适配器代码参考：
- Postgres 仓库：[postgres.go](file:///d:/dev/one-permit/permit-backend/internal/infrastructure/repo/postgres.go)
- 内存仓库（回退）：[memory.go](file:///d:/dev/one-permit/permit-backend/internal/infrastructure/repo/memory.go)

## DSN 示例
- 无 TLS（本机）：`postgres://postgres:yourpass@127.0.0.1:5432/permit?sslmode=disable`
- 必须 TLS：`postgres://postgres:yourpass@127.0.0.1:5432/permit?sslmode=require`
- 自定义端口：`postgres://user:pass@localhost:6543/permit?sslmode=disable`
- 远程主机：`postgres://user:pass@db.example.com:5432/permit?sslmode=require`

## 常见问题与排查
- 构建缺少 go.sum：运行 `go mod tidy`
- 连接拒绝：检查端口、防火墙与 DSN 主机设置
- 权限不足：为目标数据库授予读写权限
- 自动建表：仅限开发模式；生产建议使用迁移工具（goose/tern）
- 配置未生效：检查 .env/.env.local 是否被正确加载，或在当前会话设置 `$env:POSTGRES_DSN`

## 关键代码参考
- 接口适配层（Gin）：[server.go](file:///d:/dev/one-permit/permit-backend/internal/server/server.go)
- 配置加载： [config.go](file:///d:/dev/one-permit/permit-backend/internal/config/config.go)、[main.go](file:///d:/dev/one-permit/permit-backend/cmd/permit-backend/main.go)
- 仓库实现： [postgres.go](file:///d:/dev/one-permit/permit-backend/internal/infrastructure/repo/postgres.go)、[memory.go](file:///d:/dev/one-permit/permit-backend/internal/infrastructure/repo/memory.go)
