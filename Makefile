.PHONY: all fmt test check staticcheck vulncheck deadcode build

all: fmt test check build

fmt:
	go run mvdan.cc/gofumpt@latest -w ./

test:
	go test -v ./...

check: staticcheck vulncheck deadcode

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

deadcode:
	go run golang.org/x/tools/cmd/deadcode@latest -test ./...
