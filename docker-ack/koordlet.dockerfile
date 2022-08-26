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

RUN ls -al /go/src/github.com/koordinator-sh/koordinator && sleep 10
RUN go build -a -o koordlet cmd/koordlet/main.go

FROM --platform=$TARGETPLATFORM nvidia/cuda:11.6.1-base-ubuntu20.04
WORKDIR /
RUN apt-get update && apt-get upgrade -y && rm -rf /var/cache/apt/
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koordlet .
ENTRYPOINT ["/koordlet"]
