# TCP 协议驱动镜像
#
# 构建并推送（把 IMAGE 换成你自己的仓库地址）：
#   docker buildx build --platform linux/amd64 \
#     -t IMAGE -f docker/Dockerfile . --push

FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/driver ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /etc/driver/res /mnt/logs

COPY --from=builder /out/driver /app/driver

# 设备接入端口（默认 9000，可用自定义参数 ListenAddr 改；改端口后要同步映射）
EXPOSE 9000

ENTRYPOINT ["/app/driver"]