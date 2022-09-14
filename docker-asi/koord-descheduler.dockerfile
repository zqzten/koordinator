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

RUN CGO_ENABLED=0 go build -a -o koord-descheduler cmd/koord-descheduler/main.go

FROM --platform=$TARGETPLATFORM reg.docker.alibaba-inc.com/ackee/centos:latest
WORKDIR /
RUN yum install net-tools iproute bind-utils openssh-clients cronie sysvinit-tools less vim -y && yum update -y
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-descheduler .
ENTRYPOINT ["/koord-descheduler"]
