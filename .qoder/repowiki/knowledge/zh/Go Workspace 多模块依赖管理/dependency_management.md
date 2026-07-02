## 系统概览

NexTunnel 采用 Go 1.25 的 go.work 多工作区（Multi-Module Workspace）模式，将 CLI、桌面客户端、服务端、安装器与共享库五个子模块聚合到单一仓库中，通过顶层 go.work + go.work.sum 统一版本约束与依赖解析。

### 工作区结构

go.work 聚合以下模块：cli/go.mod、desktop/go.mod、installer/go.mod、pkg/go.mod、server/go.mod。所有模块声明 go 1.25.0，由根级 go.work 统一指定。

### 内部包替换策略

desktop 与 server 通过 replace 指令指向本地 ../pkg：replace github.com/nextunnel/pkg => ../pkg。这使得两个应用直接消费本地共享库，无需发布中间版本。cli 和 installer 不依赖 pkg，保持最小化。

### 依赖锁定与缓存

- go.work.sum 记录工作区内所有模块的 go.mod 校验和，确保跨模块依赖一致性
- 根目录存在 .gocache-release、.gocache-test、.gocache-test-cli、.gocache-test-desktop、.gocache-test-pkg、.gocache-test-server 六个独立 Go 构建缓存目录，分别对应不同构建/测试场景，避免缓存污染
- 未使用 vendor 目录，依赖直接从远程下载

### 前端依赖隔离

前端代码各自维护独立的 package.json + node_modules：desktop/frontend、installer/frontend、server/web。Makefile 的 install-deps 目标分别在各子目录执行 npm install，互不影响。

### 构建与依赖更新流程

顶层 Makefile 提供统一入口：make build 调用 wails build；make build-server 逐个 go build 生成各二进制；make test 遍历各模块执行 go test；make install-deps 依次在各子模块执行 go mod tidy + npm install。CI 流水线（.github/workflows/ci.yml、release.yml）复用同一套 Makefile 目标。

### 开发者约定

1. 新增内部依赖在对应子模块的 go.mod 中添加，并通过 replace 指向 ../pkg（若属于共享库）
2. 修改后运行 make install-deps，会自动调用各子模块的 go mod tidy
3. 不要提交 vendor：项目不使用 vendoring，依赖以 go.mod/go.sum 形式管理
4. Go 版本锁定：所有模块统一使用 1.25.0，禁止混用版本
5. 私有仓库：未发现 GOPRIVATE 或自定义 proxy 配置，依赖均从公共源获取