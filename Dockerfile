# 第一阶段：编译
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /cleanup .

# 第二阶段：极简运行镜像
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /cleanup /cleanup
ENTRYPOINT ["/cleanup"]
