.PHONY: run build test clean

run: build
	./kasmos $(ARGS)

build:
	go build -o kasmos .

test:
	go test ./... -v

clean:
	rm -f kasmos
