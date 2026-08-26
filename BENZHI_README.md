# BENZHI_README

## 项目说明
- 项目：benzhi-project-b124f6b6-09a0-448f-a8bd-6f32d5f55eb0
- 项目用途：已完整实现监测井退役封填的浏览器见证台，覆盖基线冻结、分层施工、偏差纠正、完整性验证、独立放行、审计链和确定性只读归档。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：监测井封填见证台
- 项目介绍：面向地下水监测井退役作业的浏览器见证应用，将封填基线、分段施工证据、偏差处置、完整性验证和独立放行串成一条可追溯且最终冻结的质量流程。
- 项目概述：面向地下水监测井退役作业的浏览器见证应用，将封填基线、分段施工证据、偏差处置、完整性验证和独立放行串成一条可追溯且最终冻结的质量流程。
- 核心工作流：封填个案由草稿建档开始，冻结井况与施工基线后进入分段封填；施工偏差会将个案置为暂停，完成纠正与复验后恢复，随后通过完整性验证、独立放行并生成只读归档，状态依次体现为 draft、baseline_locked、sealing、held、verification、released、archived。
- 对外接口：Go 服务在默认 127.0.0.1:19091 提供原生 HTML、CSS 和 JavaScript 的单页浏览器工作台及同源 JSON 接口；监听地址支持 -addr=127.0.0.1:<port>，不引入 Node 构建链，页面可完成唯一主流程、查看状态时间线并下载归档。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/wellseal -self-check -addr=127.0.0.1:19091

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-b124f6b6-09a0-448f-a8bd-6f32d5f55eb0-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-b124f6b6-09a0-448f-a8bd-6f32d5f55eb0-arm64 linux/arm64

docker run -it benzhi-project-b124f6b6-09a0-448f-a8bd-6f32d5f55eb0-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/wellseal -self-check -addr=127.0.0.1:19091`
