VERSION ?= 0.1.0-alpha.1
DIST    := dist
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all windows test vet clean

all: test windows

# Windows GUI executable, version in the file name.
windows:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
		-ldflags "$(LDFLAGS) -H=windowsgui" \
		-o $(DIST)/ptx-to-flp_v$(VERSION)_windows_amd64.exe ./cmd/ptx-to-flp

test:
	go test ./...

vet:
	go vet ./...
	GOOS=windows go vet ./cmd/...

clean:
	rm -rf $(DIST)
