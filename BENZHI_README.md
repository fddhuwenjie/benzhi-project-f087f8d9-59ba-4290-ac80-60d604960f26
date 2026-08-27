# BENZHI_README

## 项目说明
- 项目：benzhi-project-f087f8d9-59ba-4290-ac80-60d604960f26
- 项目用途：应急广播脚本演练发布台提供脚本分段编制、基线冻结、确定性校验、计时演练、整改复验、独立批准、内容寻址发布包和审计核验的完整浏览器闭环。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：应急广播脚本演练发布台
- 项目介绍：面向公共场馆应急信息团队的浏览器工作台，将一份应急广播脚本从分段编制、规则校验、计时演练、问题整改推进到独立批准，并冻结为可核验的发布包。
- 项目概述：面向公共场馆应急信息团队的浏览器工作台，将一份应急广播脚本从分段编制、规则校验、计时演练、问题整改推进到独立批准，并冻结为可核验的发布包。
- 核心工作流：编审人员创建广播方案并录入有序脚本段，冻结适用场景、目标听众、播出渠道、时限与术语基线；系统执行确定性校验后进入演练，记录每段实际耗时、口误和听辨问题；未通过项形成整改任务并在修订后定向复验；全部门禁满足时由未参与编写的批准人员签署，系统冻结内容寻址的发布包并提供完整性核验，状态依次由草拟、待校验、待演练、整改中、待批准变为已发布或已拒绝。
- 对外接口：Go 服务直接提供无需 Node 构建的原生 HTML、CSS 和 JavaScript 单页工作台；页面通过同源 JSON 端点完成脚本编辑、校验、演练记录、整改、批准、时间线查看与发布包核验，JSON 端点仅服务该浏览器界面。服务支持 -addr=127.0.0.1:<port>，默认监听 127.0.0.1:19081；PORT 为端口号时绑定 127.0.0.1:<PORT>，不得默认绑定 0.0.0.0。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/broadcastdesk -selfcheck -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-f087f8d9-59ba-4290-ac80-60d604960f26-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-f087f8d9-59ba-4290-ac80-60d604960f26-arm64 linux/arm64

docker run -it benzhi-project-f087f8d9-59ba-4290-ac80-60d604960f26-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/broadcastdesk -selfcheck -addr=127.0.0.1:19081`
