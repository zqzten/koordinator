FROM --platform=$TARGETPLATFORM golang:1.20 as builder
WORKDIR /go/src/github.com/koordinator-sh/koordinator

ARG VERSION
ARG TARGETARCH
ENV VERSION $VERSION
ENV GOOS linux
ENV GOARCH $TARGETARCH
ENV GOPRIVATE "gitlab.alibaba-inc.com"
ENV GOPROXY "https://goproxy.cn,direct"

COPY apis/ apis/
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY go.* ./

RUN git config --global url."git@gitlab.alibaba-inc.com:".insteadOf "https://gitlab.alibaba-inc.com/" && \
  mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.alibaba-inc.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh go mod download

RUN CGO_ENABLED=0 go build -a -o koord-manager ./cmd/koord-manager

RUN --mount=type=ssh mkdir -p /go/src/gitlab.alibaba-inc.com/koordinator-sh/x && \
    cd /go/src/gitlab.alibaba-inc.com/koordinator-sh/x && \
    git clone git@gitlab.alibaba-inc.com:koordinator-sh/x.git . && \
    go build -mod=vendor -v -a -o logrotate.bin logrotate/logrotate.go

FROM --platform=$TARGETPLATFORM registry-cn-hangzhou.ack.aliyuncs.com/dev/alpine:3.18-update as utils-builder

FROM --platform=$TARGETPLATFORM registry-cn-hangzhou.ack.aliyuncs.com/dev/alpine:3.18-base
# https://aliyuque.antfin.com/op0cg2/cu4mmd/ikauud
WORKDIR /
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-manager .
COPY --from=builder /go/src/gitlab.alibaba-inc.com/koordinator-sh/x/logrotate.bin ./logrotate
COPY --from=utils-builder /bin/cat /bin/
ENTRYPOINT ["./logrotate", "--file=/log/koord-manager.log"]
