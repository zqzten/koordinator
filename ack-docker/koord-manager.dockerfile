FROM --platform=$TARGETPLATFORM golang:1.17 as builder
WORKDIR /go/src/github.com/koordinator-sh/koordinator

ARG VERSION
ARG TARGETARCH
ENV VERSION $VERSION
ENV GOOS linux
ENV GOARCH $TARGETARCH

COPY apis/ apis/
COPY bin/ bin/
COPY ack-crds/ ack-crds/
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY go.* ./
COPY config/crd/bases/ koord-crds/

RUN git config --global url."git@gitlab.alibaba-inc.com:".insteadOf "https://gitlab.alibaba-inc.com/" && \
  mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.alibaba-inc.com >> ~/.ssh/known_hosts
RUN --mount=type=ssh go mod download

RUN ls -al /go/src/github.com/koordinator-sh/koordinator
RUN cp bin/kubectl-$TARGETARCH bin/kubectl
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o koord-manager cmd/koord-manager/main.go

FROM --platform=$TARGETPLATFORM alpine:3.16
RUN apk --no-cache --update upgrade && rm -rf /var/cache/apk/*
RUN apk add --update bash net-tools iproute2 logrotate less rsync util-linux lvm2
RUN addgroup -S nonroot && adduser -u 1200 -S nonroot -G nonroot
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-manager /usr/bin/koord-manager
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/start-koord-manager.sh /usr/bin/start-koord-manager.sh
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/kubectl /usr/bin/kubectl

COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/ack-crds/* /etc/koordinator-crds/
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-crds/* /etc/koordinator-crds/