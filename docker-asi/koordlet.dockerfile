FROM --platform=$TARGETPLATFORM golang:1.18 as builder
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

RUN rm -rf /etc/apt/sources.list.d/* && \
    echo 'deb http://mirrors.aliyun.com/debian/ bullseye main non-free contrib' > /etc/apt/sources.list && \
    echo 'deb-src http://mirrors.aliyun.com/debian/ bullseye main non-free contrib' >> /etc/apt/sources.list && \
    echo 'deb http://mirrors.aliyun.com/debian-security bullseye-security main non-free contrib' >> /etc/apt/sources.list && \
    echo 'deb-src http://mirrors.aliyun.com/debian-security bullseye-security main non-free contrib' >> /etc/apt/sources.list && \
    echo 'deb http://mirrors.aliyun.com/debian/ bullseye-updates main non-free contrib' >> /etc/apt/sources.list && \
    echo 'deb-src http://mirrors.aliyun.com/debian/ bullseye-updates main non-free contrib' >> /etc/apt/sources.list && \
    echo 'deb http://mirrors.aliyun.com/debian/ bullseye-backports main non-free contrib' >> /etc/apt/sources.list && \
    echo 'deb-src http://mirrors.aliyun.com/debian/ bullseye-backports main non-free contrib' >> /etc/apt/sources.list
RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys DCC9EFBF77E11517 && \
    apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 0E98404D386FA1D9

RUN apt update && apt install -y bash build-essential cmake wget
RUN wget https://sourceforge.net/projects/perfmon2/files/libpfm4/libpfm-4.13.0.tar.gz && \
  echo "bcb52090f02bc7bcb5ac066494cd55bbd5084e65  libpfm-4.13.0.tar.gz" | sha1sum -c && \
  tar -xzf libpfm-4.13.0.tar.gz && \
  rm libpfm-4.13.0.tar.gz
RUN export DBG="-g -Wall" && \
  make -e -C libpfm-4.13.0 && \
  make install -C libpfm-4.13.0
RUN go build -a -o koordlet cmd/koordlet/main.go

FROM --platform=$TARGETPLATFORM registry-cn-hangzhou.ack.aliyuncs.com/dev/ubuntu:20.04-update as utils-builder

# use a secure pruned CUDA image
FROM --platform=$TARGETPLATFORM registry.cn-hangzhou.aliyuncs.com/acs/ubuntu:20.04-base-cuda-11.6
WORKDIR /
USER 0
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koordlet .
COPY --from=builder /usr/local/lib /usr/lib
COPY --from=utils-builder /usr/bin/cat /usr/bin/
ENTRYPOINT ["/koordlet"]
