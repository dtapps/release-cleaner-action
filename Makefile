# 设置默认目标
.DEFAULT_GOAL := setup

# 环境变量优先级高于 .env：include 的赋值会覆盖环境值，先暂存一份
ifeq ($(origin GIT_REMOTE),environment)
    GIT_REMOTE_ENV := $(GIT_REMOTE)
endif

# 加载 .env 文件（如果存在）
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

ifneq ($(GIT_REMOTE_ENV),)
    GIT_REMOTE := $(GIT_REMOTE_ENV)
endif

# 同步/推送目标远程，逗号分隔：GIT_REMOTE=origin,github,gitlab
# 可通过 .env / 环境变量 / 命令行覆盖，未配置时默认 origin。
GIT_REMOTE ?= origin
export GIT_REMOTE

# 默认目标
help: ## 显示帮助信息
	@echo "CertFlow 开发命令"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: deps ## 一键设置项目（默认执行）
	@echo "🎉 Project setup completed!"

.PHONY: deps
deps: deps-go ## 安装全部依赖
	@echo "[Deps] 完成。"

.PHONY: deps-go
deps-go: ## 安装全部 Go 依赖
	@echo "[Deps] 安装 Go 模块依赖..."
	go mod tidy
	go mod download

.PHONY: update-deps
update-deps: update-deps-go ## 更新全部依赖
	@echo "[Deps] 完成。"

.PHONY: update-deps-go
update-deps-go: ## 更新全部 Go 依赖
	@echo "[Deps] 更新 Go 模块依赖..."
	go get -u ./...
	go mod tidy

#####################
## 格式化 相关
#####################

.PHONY: format-go ## 格式化后端文件
format-go:
	@echo "[Format] 格式化后端文件 (GO)..."
	gofmt -w -s .
	go fmt ./...
	go fix ./...

.PHONY: format-yaml ## 格式化 YAML 文件
format-yaml:
	@bash scripts/format.sh yaml

.PHONY: format-json ## 格式化严格 JSON 文件（gojq）
format-json:
	@bash scripts/format.sh json

.PHONY: format-json5 ## 格式化 JSON5 文件（jsonfmt，保留注释 / 无引号 key / 尾逗号）
format-json5:
	@bash scripts/format.sh json5

.PHONY: format-sql ## 格式化 SQL 文件
format-sql:
	@bash scripts/format.sh sql

.PHONY: format-markdown ## 格式化 Markdown 文件
format-markdown:
	@bash scripts/format.sh markdown

.PHONY: format-docker ## 格式化 Dockerfile 文件
format-docker:
	@bash scripts/format.sh docker

.PHONY: format-shell ## 格式化 Shell 脚本
format-shell:
	@bash scripts/format.sh shell

.PHONY: format ## 格式化/修复项目
format: format-go format-yaml format-json format-json5 format-sql format-markdown format-docker format-shell

#####################
## 检查 / 测试 相关
#####################

# 统一定义构建命令
# CGO_ENABLED=0: 禁用 CGO，实现完全静态链接
# CGO_ENABLED=1: 启用 CGO，实现动态链接
LINT_ENV = CGO_ENABLED=0

# 定义 Lint 过程中需要激活的 Go Build Tags
LINT_TAGS = goolm,stdjson,purego

# Lint 扫描的目标平台，默认当前平台；CI 通过 LINT_PLATFORMS=... 覆盖为全平台
LINT_PLATFORMS ?= $(shell go env GOOS)/$(shell go env GOARCH)

# -printf=false 关闭 printf 分析，避免 i18n.T() 作日志 format 时触发错误
.PHONY: check-go-vet
check-go-vet: ## 运行 Go 静态检查
	@echo "[Check] -> 运行 Go vet 静态检查 (Tags: $(LINT_TAGS))..."
	$(LINT_ENV) go vet -tags=$(LINT_TAGS) -printf=false ./...

.PHONY: check-go-lint
check-go-lint: ## 运行 Go lint 扫描
	@echo "[Check] -> 运行 Go lint 扫描 (平台: $(LINT_PLATFORMS), Tags: $(LINT_TAGS))..."
	@for os_arch in $(LINT_PLATFORMS); do \
		os=$$(echo $$os_arch | cut -d'/' -f1); \
		arch=$$(echo $$os_arch | cut -d'/' -f2); \
		echo "  -> $$os 平台 ($$arch)"; \
		$(LINT_ENV) GOOS=$$os GOARCH=$$arch golangci-lint fmt; \
		$(LINT_ENV) GOOS=$$os GOARCH=$$arch golangci-lint run --build-tags $(LINT_TAGS); \
	done

.PHONY: check-go-fix
check-go-fix: ## 运行 Go fix 问题
	@echo "[Check] -> 运行 Go fix 问题 (Tags: $(LINT_TAGS))..."
	$(LINT_ENV) golangci-lint run --fix --build-tags $(LINT_TAGS)

.PHONY: check-go-test
check-go-test: ## 运行 Go 单元测试
	@echo "[Check] -> 运行 Go 单元测试 (Tags: $(LINT_TAGS))..."
	$(LINT_ENV) go test -tags=$(LINT_TAGS) $(GO_COVER_TEST_FLAGS) $(GO_COVER_SCAN)

.PHONY: check-go-vuln
check-go-vuln: ## 运行 Go 依赖漏洞检查
	@echo "[Check] -> 运行 Go 依赖漏洞检查 (Tags: $(LINT_TAGS))..."
	$(LINT_ENV) govulncheck -tags=$(LINT_TAGS) -scan=package -show verbose ./...

.PHONY: check-go
check-go: check-go-vet check-go-lint check-go-fix check-go-test check-go-vuln ## 检查后端项目

.PHONY: check
check: check-go ## 检查/测试项目

#####################
## 更新 / 拉取 / 推送 相关
#####################

.PHONY: sync
sync: ## 按 GIT_REMOTE 顺序先全部拉取合并，再统一推送（保留本地未提交改动）
	@IFS=,; for r in $$GIT_REMOTE; do \
		git fetch "$$r" && git merge --ff-only "$$r/main" || exit 1; \
	done
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "同步完成！"

.PHONY: push
push: ## 推送到所有远程仓库
	@IFS=,; for r in $$GIT_REMOTE; do git push "$$r" HEAD || exit 1; done
	@echo "推送完成！"

.PHONY: push-force
push-force: ## 强制推送到所有远程仓库（忽略冲突）
	@IFS=,; for r in $$GIT_REMOTE; do git push --force "$$r" HEAD || exit 1; done
	@echo "强制推送完成！"

#####################
## 测试覆盖率 相关
#####################

# ── 清单配置：新增目录 / 生成代码只改这里，下面的命令不散落路径 ──────────────
# Go 扫描范围：业务代码全在 internal/ 下，cmd/ 只是可执行入口，不计入分母。
GO_COVER_SCAN = ./...
# Go 排除目录（包路径前缀，空格分隔）：计入会把指标压到接近 0。
GO_COVER_EXCLUDE = 
# 关闭 vet 检查，加快测试与覆盖率统计速度
GO_COVER_TEST_FLAGS = -vet=off

# -coverpkg 只收「扫描范围内且不在排除列表」的包。用 $$(...) 让 go list 在配方执行时才跑，
# 避免任何 make 目标都触发一次包扫描；grep 参数由排除列表生成，加目录不用改命令。
GO_COVER_EXCLUDE_ARGS = $(foreach d,$(GO_COVER_EXCLUDE),-e '$(d)')
GO_COVER_PKGS = $$(go list $(GO_COVER_SCAN) | grep -v $(GO_COVER_EXCLUDE_ARGS) | paste -sd, -)
GO_COVER_ARGS = $(GO_COVER_TEST_FLAGS) -coverpkg=$(GO_COVER_PKGS) -coverprofile=coverage.out $(GO_COVER_SCAN)

.PHONY: cover-go
cover-go: ## Go 覆盖率（golang 格式）→ coverage.out；CNB 的 testing:coverage 与 Codecov 都读这份
	@echo "[Coverage] Go 测试覆盖率 → coverage.out"
	go test $(GO_COVER_ARGS)
	go tool cover -func=coverage.out

.PHONY: junit-go
junit-go: ## Go 覆盖率 + JUnit（Codecov Test Analytics 用），依赖 CI 镜像预装的 gotestsum
	@echo "[Coverage] Go 测试 → coverage.out + go-test-report.xml"
	gotestsum --junitfile go-test-report.xml -- $(GO_COVER_ARGS)
	go tool cover -func=coverage.out

# gocover-cobertura 由统一 CI 镜像预装；-strict 让「包/模块信息缺失被静默跳过」直接失败。
.PHONY: cover-go-cobertura
cover-go-cobertura: cover-go ## Coveralls 的 golang parser 会把包前缀压成 basename，改用 Cobertura 保留完整 filename 路径
	@echo "[Coverage] coverage.out → coverage.xml（Cobertura）"
	gocover-cobertura -by-files -strict < coverage.out > coverage.xml
