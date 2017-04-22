ARTIFACT = parser 

all: build

build: GOOS ?= linux
build: GOARCH ?= amd64
build: clean
		GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0 go build -o ${ARTIFACT} -a .

clean:
		rm -rf ${ARTIFACT}

image: clean
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ${ARTIFACT} -a .

test:
		go test -v

run: build
	./${ARTIFACT}
