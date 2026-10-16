# TCP 协议驱动镜像
#
# 本地构建：
#   make build IMAGE=iotserver-tcp-driver:latest
# 推送（CI 会自动做，见 .github/workflows/docker-publish.yml）：
#   make push IMAGE=ghcr.io/lmgateways/iotserver-tcp-driver:v1.0.0

FROM golang:1.24-alpine AS builder

# 拉 Go 依赖用的代理：国内网络用 goproxy.cn，CI 环境可用默认（proxy.golang.org）
ARG GOPROXY=https://goproxy.cn,direct

WORKDIR /src

# 先拷依赖清单，利用层缓存：只改代码时不重下依赖
COPY go.mod go.sum ./
RUN go env -w GOPROXY=${GOPROXY} && go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/driver ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /etc/driver/res /mnt/logs

COPY --from=builder /out/driver /app/driver

# 设备接入端口（默认 9000，可用自定义参数 ListenAddr 改；改端口后要同步映射）
EXPOSE 9000

ENTRYPOINT ["/app/driver"]