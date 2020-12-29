.PHONY: all test coverage

all: get build install

get:
	go get ./...

build:
	cd cmd/hyperarp && config build -v --versiondir='../../' --plattforms='linux/amd64' && env GOOS=windows GOARCH=386 CGO_ENABLED=1 CXX=i686-w64-mingw32-g++ CC=i686-w64-mingw32-gcc go build .

release:
	config release --versiondir='.' --package='hyperarp'
	cd cmd/hyperarp && config build -v --versiondir='../../' --plattforms='linux/amd64' && env GOOS=windows GOARCH=386 CGO_ENABLED=1 CXX=i686-w64-mingw32-g++ CC=i686-w64-mingw32-gcc go build .

test:
	go test ./... -v -coverprofile .coverage.txt
	go tool cover -func .coverage.txt

coverage: test
	go tool cover -html=.coverage.txt
