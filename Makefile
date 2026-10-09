vet:
	go vet ./...

fmt:
	go fmt ./...

lint:
	golangci-lint run

test:
	go test -v ./...

build:
	go build -o bin/pegg cmd/pegg/main.go

run:
	./bin/pegg

clean:
	rm -rf bin