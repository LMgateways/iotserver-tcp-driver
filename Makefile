.PHONY: build push tidy

# 镜像地址：用 IMAGE 覆盖，例如
#   make build IMAGE=registry.example.com/iotserver-tcp-driver:v1.0.0
IMAGE ?= iotserver-tcp-driver:latest

build:
	docker buildx build --platform linux/amd64 -t $(IMAGE) -f docker/Dockerfile . --load

push:
	docker buildx build --platform linux/amd64 -t $(IMAGE) -f docker/Dockerfile . --push

tidy:
	go mod tidy
	go build ./...