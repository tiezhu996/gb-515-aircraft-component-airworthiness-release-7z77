# 验证记录

验证日期：2026-08-22（Asia/Shanghai）

## 部件装配关系与逐级核对（2026-09-25 追加）

在 SQLite 开发模式（`DATABASE_DRIVER=sqlite`、`REDIS_ADDR=''`）下实际验证：

- `go test ./...`、`go test -race ./...`、`go vet ./...`、前端 `npm run typecheck` / `npm run build` 全部通过；新增装配服务测试覆盖自挂自、重复登记、第二个父件、多层绕回、逐级核对和批准拦截。
- 空库种子自动建立 AP-004（组件）→ AP-005/006/007 子件与 AP-005 → AP-008 孙件；RA-002 关联 AP-004。
- `GET /api/part-assembly/4` 返回装配清单与核对结果：`ready=false, checked=4`，卡住编号 AP-006(hold/暂停 L1)、AP-007(retired/退役 L1)、AP-008(inspection/未放行 L2)。
- 登记接口对自挂自、重复关系、同一子件挂第二父件、绕回祖先均返回 HTTP 409 `assembly_conflict`；viewer 登记返回 403。
- reviewer 批准 RA-002（approved 与 restricted 两种路径）均返回 HTTP 409 `assembly_release_blocked`，响应体 `blockedParts` 列出三个编号；授权保持 `review`、版本仍为 1、版本快照不增加，审计写入 `release-blocked`。
- 将 AP-006/AP-008 推进到 released、拆除 AP-007 后，`GET /api/part-assembly/4` 变为 `ready=true, checked=3`；reviewer 再次批准成功，RA-002 变为 approved v2。
- 草稿授权 PUT 绑定/解绑 `aircraftPartId` 正常；绑定不存在的部件返回 404。
- 登记与拆除装配关系均写审计（`register` / `remove`，含操作者与部件编号）。

## 代码质量

以下命令均实际执行成功：

```bash
cd backend
gofmt -w <本次修改的 Go 文件>
go test ./...
go test -race ./...
go vet ./...
go build ./...

cd ../frontend
npm run typecheck
npm run build

cd ..
docker compose config --quiet
```

- 非测试 Go 代码：3239 行。
- 非测试 `.go` 文件：38 个。
- 服务层回归测试覆盖证书异人发布、放行双人复核、operator 越权、同人伪装 reviewer、复核后编辑锁定、版本与审计数量。

## 空卷 Compose 与 API

执行 `KEEP_RUNNING=1 ./scripts/validate.sh`，脚本先运行 `docker compose down -v --remove-orphans`，再从空命名卷构建并启动 MySQL、Redis、MinIO、backend、frontend。验证结果：

- `/healthz` 返回 database=`ready`、redis=`ready`。
- `viewer` 可读取部件与会话，POST 写入返回 HTTP 403。
- `operator` 创建放行草稿 v1，并提交为 review v2；其自批请求返回 HTTP 403。
- `reviewer` 独立批准为 v3，`submittedBy=operator`、`reviewedBy=reviewer`。
- 三个放行版本分别保留 actor、request ID、状态、证据和原因。
- `operator` 创建证书 v1；其发布请求返回 HTTP 403；`reviewer` 发布为 valid v2。
- 证书版本保留 `preparedBy=operator`、`verifiedBy=reviewer` 和请求 ID。
- 实体审计历史包含 `gb515-auth-create`、`gb515-auth-review`、`gb515-auth-approve`；审计汇总覆盖两个独立操作者。

## 内置 Browser

仅使用 Codex 内置 Browser，在 `http://127.0.0.1:18515` 实际验证：

- 登录页可选择 admin/reviewer/operator/viewer 并建立真实 JWT 会话。
- `/parts`、`/inspections`、`/certificates`、`/authorizations`、`/audit` 五个页面均加载成功。
- 在部件页通过确认对话框将 AP-001 从 received 推进到 inspection，页面刷新后状态正确。
- `PartStatusBadge` 在部件与放行页渲染；`CertificatePanel` 在检查与证书页展示版本、操作者和请求 ID。
- operator 在 review/approved 放行记录上只看到“等待复核员”，没有批准按钮；draft 仍可提交 review。
- viewer 不显示新增和状态推进按钮，只显示只读状态。
- 审计页展示 operator/reviewer 的创建、提交、批准、发布动作及对应请求 ID。
- 桌面全页截图和 390 x 844 移动端截图已检查；移动端表格采用稳定横向滚动，不挤压文字。
- 最终控制台 `error`/`warning` 日志为空。

## 清理

验证结束后执行：

```bash
docker compose down -v --remove-orphans
```

并确认没有名称包含 `aircraft-component-airworthiness-release` 的运行中容器。
