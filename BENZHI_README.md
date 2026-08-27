基于 Go 实现的建筑幕墙结构胶质量闭环 Web 项目，一款后端服务，管理单元板块从基材相容性确认、连续注胶、养护试验、异常返工到吊装准入裁定的只追加证据链。

# unitized-curtainwall-silicone-hoist-gate

本 Git 项目来自模型完成任务后的 workspace，不包含嵌套 .git 记录或本地构建产物。

## 本地构建与测试

```bash
go mod download
go build ./...
go test ./...
./run_benzhi_smoke.sh
```

## Docker 构建与运行

```bash
docker build --platform linux/amd64 -t unitized-curtainwall-silicone-hoist-gate:latest .
./build_benzhi_docker.sh unitized-curtainwall-silicone-hoist-gate linux/arm64
docker run --rm -it --platform linux/arm64 unitized-curtainwall-silicone-hoist-gate:latest
./build_benzhi_docker.sh unitized-curtainwall-silicone-hoist-gate linux/amd64
docker run --rm -it --platform linux/amd64 unitized-curtainwall-silicone-hoist-gate:latest
```

构建脚本第二个参数为目标平台，必须分别完成 linux/arm64 和 linux/amd64 构建与容器验证；未提供时按照规范默认使用 linux/amd64。系统 backend-v2 模板通过 Go 原生交叉编译生成目标架构的 /usr/local/bin/benzhi-app，镜像默认直接运行该入口。
