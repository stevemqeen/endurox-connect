export GOBIN?=$(shell pwd)

BINARY=${GOBIN}/restoutsv

all:
	go build ${LDFLAGS} -o ${BINARY}

clean:
	rm -f ${BINARY}
