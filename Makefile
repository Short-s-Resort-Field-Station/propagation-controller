.PHONY: build test run-sim linux clean web

BIN := bin

build:
	go build -o $(BIN)/propagationd ./cmd/propagationd

# Build the frontend and embed it into the binary's webdist.
web:
	cd web && npm install --silent && npm run build
	rm -rf internal/api/webdist
	mkdir -p internal/api/webdist
	cp -r web/dist/* internal/api/webdist/
	$(MAKE) build

test:
	go test ./...

# Development on any machine, no hardware required (spec §24.4).
run-sim: build
	$(BIN)/propagationd -simulation -log-pretty -log-level debug -data-dir ./data

# Cross-compile for the Piconium (x86_64, static — no CGo thanks to modernc sqlite).
linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN)/propagationd-linux-amd64 ./cmd/propagationd
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN)/propagation-watchdog-linux-amd64 ./cmd/propagation-watchdog
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN)/probe-address-linux-amd64 ./cmd/probe-address

clean:
	rm -rf $(BIN) data
