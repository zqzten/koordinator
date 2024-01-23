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
COPY vendor/ vendor/
COPY config/crd/bases/ koord-crds/
COPY cnstack-rbac/ koord-rbac/

#RUN git config --global url."git@gitlab.alibaba-inc.com:".insteadOf "https://gitlab.alibaba-inc.com/" && \
#  mkdir -p -m 0600 ~/.ssh && ssh-keyscan gitlab.alibaba-inc.com >> ~/.ssh/known_hosts
#RUN --mount=type=ssh go mod download

RUN ls -al /go/src/github.com/koordinator-sh/koordinator
RUN cp bin/kubectl-$TARGETARCH bin/kubectl
RUN cp bin/logrotate-$TARGETARCH bin/logrotate

RUN CGO_ENABLED=0 GOOS=linux go build -a -o koord-scheduler ./cmd/koord-scheduler


FROM --platform=$TARGETPLATFORM alpine:3.16
WORKDIR /
RUN apk --no-cache --update upgrade && rm -rf /var/cache/apk/*
RUN apk add --update bash net-tools iproute2 logrotate less rsync util-linux lvm2
RUN addgroup -S nonroot && adduser -u 1200 -S nonroot -G nonroot
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-scheduler .
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/logrotate ./logrotate
COPY cnstack-docker/start-cnstack-koord-scheduler.sh .

COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/bin/kubectl /usr/bin/kubectl
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/ack-crds/* /etc/koordinator-crds/
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-crds/* /etc/koordinator-crds/
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-rbac/* /etc/koordinator-rbac/