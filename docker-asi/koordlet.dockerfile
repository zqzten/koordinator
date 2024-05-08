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

#FROM --platform=$TARGETPLATFORM registry-cn-hangzhou.ack.aliyuncs.com/dev/ubuntu:20.04-update as utils-builder
# use a secure pruned CUDA image
#FROM --platform=$TARGETPLATFORM registry.cn-hangzhou.aliyuncs.com/acs/ubuntu:20.04-base-cuda-11.6
#WORKDIR /

FROM --platform=$TARGETPLATFORM ubuntu:22.04 as utils-builder
RUN apt-get update && apt-get install -y lvm2

FROM --platform=$TARGETPLATFORM nvidia/cuda:11.8.0-base-ubuntu22.04
RUN set -eux; \
    rm /etc/apt/sources.list.d/* && \
    apt-get update && apt-get upgrade -y && \
    rm -rf /var/cache/apt/ && \
    echo "only include root and nobody user" && \
    echo "root:x:0:0:root:/root:/bin/bash\nnobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin" | tee /etc/passwd && \
    echo "root:x:0:\nnogroup:x:65534:" | tee /etc/group && \
    echo "remove bins" && \
    cp /bin/rm /rm_bak && \
    /rm_bak -rf /usr/lib/apt/apt-helper  && \
    /rm_bak -rf /var/lib/dpkg/info/bash.preinst  && \
    /rm_bak -rf /usr/lib/apt/methods/*  && \
    /rm_bak -rf /usr/local/sbin/* && \
    /rm_bak -rf /usr/local/bin/* && \
    /rm_bak -rf /usr/sbin/* && \
    /rm_bak -rf /usr/bin/* && \
    /rm_bak -rf /bin/* && \
    /rm_bak -rf /sbin/* && \
    /rm_bak -rf /rm_bak

USER 0
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koordlet .
COPY --from=builder /usr/local/lib /usr/lib
COPY --from=utils-builder /usr/bin/cat /usr/bin/
ENTRYPOINT ["/koordlet"]
