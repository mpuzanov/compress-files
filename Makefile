APP := compress-files.exe

.PHONY: all fmt test vet check build clean

all: check build

fmt:
	go fmt ./...

test:
	go test ./...

vet:
	go vet ./...

check: fmt test vet

build:
	go build -trimpath -ldflags="-s -w" -o $(APP) .

clean:
	powershell -NoProfile -Command "Remove-Item -LiteralPath '.\\$(APP)' -Force -ErrorAction SilentlyContinue"
