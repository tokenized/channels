
all: clean deps tools test

clean:
	go clean -testcache

deps:
	go get -t ./...

tools:
	go get golang.org/x/tools/cmd/goimports
	go get github.com/golang/lint/golint

prepare:
	mkdir -p tmp

test: prepare
	go test -coverprofile=tmp/coverage.out ./...

test-race:
	go test -race ./...

bench:
	go test -bench . ./...
