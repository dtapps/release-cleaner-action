# 第一阶段：编译
FROM ghcr.io/dtapps/golang:latest AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /release-cleaner .

# 第二阶段：极简运行镜像
FROM ghcr.io/dtapps/alpine:latest
COPY --from=builder /release-cleaner /release-cleaner
ENTRYPOINT ["/release-cleaner"]
