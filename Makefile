build: pb
	go build -o cangling-agent main.go
pb: ./proto/agent.proto
	protoc --go_out=./ --go_opt=paths=import \
        --go-grpc_out=./ --go-grpc_opt=paths=import \
        ./proto/agent.proto
