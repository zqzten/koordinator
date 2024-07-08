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

RUN git config --global url."git@gitlab.alibaba-inc.com:".insteadOf "https://gitlab.alibaba-inc.com/" && \
  mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.alibaba-inc.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh go mod download

RUN ls -al /go/src/github.com/koordinator-sh/koordinator
RUN CGO_ENABLED=0 go build -a -o koord-scheduler ./cmd/koord-scheduler

RUN --mount=type=ssh mkdir -p /go/src/gitlab.alibaba-inc.com/unischeduler/x && \
    cd /go/src/gitlab.alibaba-inc.com/unischeduler/x && \
    git clone git@gitlab.alibaba-inc.com:unischeduler/x.git . && \
    go build -mod=vendor -v -a -o logrotate.bin logrotate/logrotate.go

FROM --platform=$TARGETPLATFORM alpine:3.16
WORKDIR /
RUN apk --no-cache --update upgrade && rm -rf /var/cache/apk/*
RUN apk add --update bash net-tools iproute2 logrotate less rsync util-linux lvm2
RUN addgroup -S nonroot && adduser -u 1200 -S nonroot -G nonroot
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-scheduler .
COPY --from=builder /go/src/gitlab.alibaba-inc.com/unischeduler/x/logrotate.bin ./logrotate
COPY ack-docker/start-koord-scheduler.sh .
