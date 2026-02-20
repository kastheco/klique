.PHONY: run build test clean

run: build
	./klique $(ARGS)

build:
	go build -o klique .

test:
	go test ./... -v

clean:
	rm -f klique
