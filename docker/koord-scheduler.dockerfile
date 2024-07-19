#FROM golang:1.17 as builder
FROM ant-cnstack-registry.cn-hangzhou.cr.aliyuncs.com/adp-7abed7fbca/intelligent-computing/golang:1.17 as builder

WORKDIR /go/src/github.com/koordinator-sh/koordinator

COPY go.mod go.mod
COPY go.sum go.sum

COPY apis/ apis/
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY vendor/ vendor/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64  \
    GO111MODULE=on GOPROXY=https://goproxy.cn,direct \
    go build -a -mod=vendor -o koord-descheduler cmd/koord-descheduler/main.go

FROM reg.docker.alibaba-inc.com/jiming/alpine-network:3.14_9521469
WORKDIR /
COPY --from=builder /go/src/github.com/koordinator-sh/koordinator/koord-descheduler .

ENTRYPOINT ["/koord-descheduler"]
