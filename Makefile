.PHONY: build test tol tolang lua54-subset-test

build:
	./_tools/go-inline *.go && go fmt . &&  go build

tol: *.go pm/*.go cmd/tolang/*.go
	./_tools/go-inline *.go && go fmt . && go build -o tol ./cmd/tolang

tolang: *.go pm/*.go cmd/tolang/*.go
	./_tools/go-inline *.go && go fmt . && go build -o tolang ./cmd/tolang

test:
	./_tools/go-inline *.go && go fmt . &&  go test

lua54-subset-test:
	./_tools/run-lua54-subset-tests.sh
