export GOBIN?=$(shell pwd)

BINARY=${GOBIN}/tcpgatesv

all:
	go build ${LDFLAGS} -o ${BINARY}

clean:
	rm -f ${BINARY}
