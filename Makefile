BINDIR ?= ~/.local/bin
CMDS := cmdproxy-shim cmdproxy-server
SRCS := $(shell find cmd internal -name '*.go') go.mod go.sum

all: $(CMDS)

cmdproxy-%: $(SRCS)
	go build -o $@ ./cmd/$@

install: all
	install -d $(BINDIR)
	install -m 755 $(CMDS) $(BINDIR)

test:
	go test ./...

clean:
	rm -f $(CMDS)

.PHONY: all install test clean
