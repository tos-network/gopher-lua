.PHONY: build test tolang lua54-subset-test

build:
	./_tools/go-inline *.go && go fmt . &&  go build

tolang: *.go pm/*.go cmd/tolang/tolang.go
	./_tools/go-inline *.go && go fmt . && go build cmd/tolang/tolang.go

test:
	./_tools/go-inline *.go && go fmt . &&  go test

lua54-subset-test:
	./_tools/run-lua54-subset-tests.sh
