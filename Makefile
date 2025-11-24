build: pb java-client
	go build -o cangling-agent main.go
pb: ./proto/agent.proto
	protoc --go_out=./ --go_opt=paths=import \
        --go-grpc_out=./ --go-grpc_opt=paths=import \
        ./proto/agent.proto
java-client: ./proto/agent.proto
	mvn clean package install -f java-client/pom.xml