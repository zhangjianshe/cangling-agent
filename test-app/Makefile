build: main
	docker build -t hub.cangling.cn/cangling/agent-test:1.0 .
main: src/main.c
	cc -static -o main src/main.c
clean:
	rm main