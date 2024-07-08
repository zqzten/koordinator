FROM --platform=$TARGETPLATFORM golang:1.17 as builder
WORKDIR /go/src/github.com/koordinator-sh/koordinator

ARG VERSION
ARG TARGETARCH
ENV VERSION $VERSION
ENV GOOS linux
ENV GOARCH $TARGETARCH

COPY apis/ apis/
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY go.* ./
COPY bin/ bin/

RUN git config --global url."git@gitlab.alibaba-inc.com:".insteadOf "https://gitlab.alibaba-inc.com/" && \
  mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.alibaba-inc.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh go mod download

RUN ls -al /go/src/github.com/koordinator-sh/koordinator
RUN go build -a -o koordlet cmd/koordlet/main.go

FROM --platform=$TARGETPLATFORM nvidia/cuda:11.6.1-base-ubuntu20.04
WORKDIR /
ENV NVIDIA_VISIBLE_DEVICES=""
RUN apt-get update && apt-get upgrade -y numactl libjson-c-dev jq && rm -rf /var/cache/apt/
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koordlet .
ENTRYPOINT ["/koordlet"]

COPY --from=registry.cn-hangzhou.aliyuncs.com/acs/accel-config:ubuntu-v0.0-87e1449-aliyun /usr/lib64/libaccel-config.so.1.0.0 /lib/
COPY --from=registry.cn-hangzhou.aliyuncs.com/acs/accel-config:ubuntu-v0.0-87e1449-aliyun /usr/bin/accel-config /usr/bin/
RUN ln -s /lib/libaccel-config.so.1.0.0 /lib/libaccel-config.so.1

COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/enable-dsa.sh /usr/bin/enable-dsa.sh
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/disable-dsa.sh /usr/bin/disable-dsa.sh
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/conf/ /usr/bin/conf/