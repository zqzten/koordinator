package cachepod

//go:generate bash -c "protoc -I ../../vendor -I . api.proto --go_opt=paths=source_relative --go_out=. --go-grpc_opt=paths=source_relative,require_unimplemented_servers=false --go-grpc_out=. --proto_path=$GOPATH/src/ --proto_path=$GOPATH/pkg/mod/ --proto_path=$GOPATH/src/github.com/koordinator-sh/koordinator/"
