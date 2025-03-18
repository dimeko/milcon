.PHONY: bs, bc, rs, rc
GO=go
BIN_DIR=bin

bs:
	$(GO) build -o $(BIN_DIR)/server cmd/server/main.go

bc:
	$(GO) build -o $(BIN_DIR)/client cmd/client/main.go

rs: bs
	./bin/server

rc: bc
	./bin/client -v -n $(CONN_NUM)