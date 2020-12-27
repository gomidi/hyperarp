.PHONY: all test coverage

all: get build install

get:
	go get ./...

build:
	cd cmd/hyperarp && config build -v --versiondir='../../' --plattforms='linux/amd64' && config build --versiondir='../../' --plattforms='linux/amd64'

release:
	config release --versiondir='.' --package='hyperarp'
	cd cmd/hyperarp && config build -v --versiondir='../../' --plattforms='linux/amd64' && config build --versiondir='../../' --plattforms='linux/amd64'

test:
	go test ./... -v -coverprofile .coverage.txt
	go tool cover -func .coverage.txt

coverage: test
	go tool cover -html=.coverage.txt