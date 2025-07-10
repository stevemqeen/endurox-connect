export GOBIN?=$(shell pwd)

BINARY=${GOBIN}/restincl

all:
	go build ${LDFLAGS} -o ${BINARY}

clean:
	rm -f ${BINARY}
