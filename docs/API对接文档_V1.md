# 一照通前后端对接 API 文档（V1）

## 总览
- Base Path：`/api`
- 数据格式：`application/json; charset=utf-8`
- 时间：ISO8601（UTC）
- ID：不透明字符串（随机16字节十六进制）

## 认证与鉴权
- 认证方式：Bearer Token（JWT）
- Header：`Authorization: Bearer <token>`
- V1 开发阶段可放宽：未登录允许上传与任务创建；上线需收紧

## 错误与状态
- 成功：2xx；客户端错误：4xx；服务错误：5xx
- 统一错误体：
  ```json
  {"error":{"code":"BadRequest","message":"描述","requestId":"xxx"}}
  ```

## 枚举与状态
- 任务状态：`queued | processing | done | failed`
- 订单状态：`created | pending | paid | canceled | refunded`
- 下载授权状态：`active | used | expired | revoked`

## 端点定义

### 1. 获取规格
- `GET /api/specs`
- 响应示例：
```json
[
  {"code":"passport","name":"护照","widthPx":354,"heightPx":472,"dpi":300,"bgColors":["white","blue","red"]}
]
```

### 2. 上传原图
- `POST /api/upload`
- 请求：`multipart/form-data`，字段 `file`
- 响应：
```json
{"objectKey":"uploads/ef71cb305861f4cf_test0.jpg"}
```

### 3. 创建并处理任务（V1 同步）
- `POST /api/tasks`
- 请求：
```json
{
  "specCode":"passport",
  "sourceObjectKey":"uploads/ef71cb305861f4cf_test0.jpg",
  "colors":["white","blue","red"],
  "widthPx":295,
  "heightPx":413,
  "dpi":300
}
```
- 成功响应：
```json
{
  "id":"8d1587000cab594ecd6b0ddc213866e0",
  "userId":"dev-user",
  "specCode":"passport",
  "sourceObjectKey":"uploads/ef71cb305861f4cf_test0.jpg",
  "status":"done",
  "processedUrls":{
    "blue":"/assets/8d1587000cab594ecd6b0ddc213866e0/blue.jpg",
    "white":"/assets/8d1587000cab594ecd6b0ddc213866e0/white.jpg"
  },
  "createdAt":"2026-01-30T22:58:22.355Z",
  "updatedAt":"2026-01-30T22:58:22.853Z"
}
```
- 失败响应：
```json
{
  "id":"f19aaeaf4873229dcbad5d0ee202be05",
  "status":"failed",
  "errorMsg":"algo service error ...",
  "processedUrls":{}
}
```

### 4. 查询任务
- `GET /api/tasks/{id}`
- 响应：
```json
{
  "id":"...",
  "status":"processing",
  "processedUrls":{},
  "createdAt":"...",
  "updatedAt":"..."
}
```

### 5. 下载产物信息
- `GET /api/download/{taskId}`
- 响应：
```json
{"taskId":"...","urls":{"blue":"/assets/{taskId}/blue.jpg"},"expiresIn":600}
```

### 6. 静态产物访问（开发模式）
- `/assets/{taskId}/{color}.jpg`
- 说明：直接访问生成图片；生产建议改为带签名下载

### 7. 订单创建（V1 简化）
- `POST /api/orders`
- 请求：
```json
{
  "taskId":"...",
  "items":[{"type":"electronic","qty":1},{"type":"layout","qty":1}],
  "city":"广州",
  "remark":"",
  "amountCents":2500,
  "channel":"wechat"
}
```
- 响应：
```json
{"orderId":"...","status":"created"}
```

### 8. 支付下单（V1 简化）
- `POST /api/pay/wechat`
- `POST /api/pay/douyin`
- 请求：
```json
{"orderId":"..."}
```
- 响应（示例，开发阶段可使用 mock）：
```json
{"orderId":"...","payParams":{"type":"mock","nonceStr":"mock-nonce","timeStamp":"1738425600","signType":"MD5","paySign":"mock-sign"}}
```

### 9. 支付回调（V1 简化）
- `POST /api/pay/callback`
- 行为：更新订单状态为 `paid` 或 `pending`；开发阶段可跳过验签（`signature_ok=true`）
- 请求（示例）：
```json
{"orderId":"...","status":"paid","raw":"...","signature_ok":true}
```
- 响应：
```json
{"ok":true}
```

### 10. 下载授权（预留）
- `POST /api/download/token`
- 请求：
```json
{"taskId":"...","ttlSeconds":600}
```
- 响应：
```json
{"token":"...","expiresAt":"..."}
```
- `GET /api/download/file?token=...`
- 响应：临时下载 URL 或文件流

### 11. 订单列表（V1）
- `GET /api/orders`
- 响应：
```json
{
  "items":[
    {"orderId":"...","taskId":"...","items":[{"type":"electronic","qty":1}],"amountCents":2500,"channel":"wechat","status":"paid","createdAt":"...","updatedAt":"..."}
  ],
  "page":1,"pageSize":20,"total":1
}
```

### 12. 订单详情（可选）
- `GET /api/orders/{id}`
- 响应：
```json
{"orderId":"...","taskId":"...","items":[{"type":"layout","qty":1}],"amountCents":2500,"channel":"wechat","status":"pending","createdAt":"...","updatedAt":"..."}
```

## 分页与过滤（用于任务/订单列表）
- 任务列表 Query：`?page=1&pageSize=20&status=done&specCode=passport`
- 订单列表 Query：`?page=1&pageSize=20&status=paid&channel=wechat`

## 接口实例（WeChat 优先，最简可跑）

### 创建订单（curl）
```bash
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -d '{
    "taskId":"8d1587000cab594ecd6b0ddc213866e0",
    "items":[{"type":"electronic","qty":1},{"type":"layout","qty":1}],
    "city":"广州",
    "remark":"",
    "amountCents":2500,
    "channel":"wechat"
  }'
```

### 支付下单（curl）
```bash
curl -X POST http://localhost:8080/api/pay/wechat \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: dev-123456" \
  -d '{"orderId":"ORDER-001"}'
```

### 模拟支付回调（开发阶段）
```bash
curl -X POST http://localhost:8080/api/pay/callback \
  -H "Content-Type: application/json" \
  -d '{"orderId":"ORDER-001","status":"paid","raw":"mock","signature_ok":true}'
```

## 幂等与回调约定
- Header：`Idempotency-Key`（支付/回调必传）
- 回调：原始 payload 入库，字段 `signature_ok` 表明验签结果

