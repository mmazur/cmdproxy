BINDIR ?= ~/.local/bin
CMDS := cmdproxy-shim cmdproxy-server

all: $(CMDS)

cmdproxy-%: cmd/cmdproxy-%/main.go
	go build -o $@ ./cmd/$@

install: all
	install -d $(BINDIR)
	install -m 755 $(CMDS) $(BINDIR)

test:
	go test ./...

clean:
	rm -f $(CMDS)

.PHONY: all install test clean
