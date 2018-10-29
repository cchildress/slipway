GO = go
BUILD = $(GO) build
FMT = $(GO) fmt
TEST = $(GO) test
TARGET = slipway

default: darwin64 linux64

darwin64:
	make test
	GOOS=darwin GOARCH=amd64 $(BUILD) -o $(TARGET)_darwin64 $(TARGET).go

linux64:
	make test
	GOOS=linux GOARCH=amd64 $(BUILD) -o $(TARGET)_linux64 $(TARGET).go

fmt:
	$(FMT) $(TARGET).go

test:
	$(TEST)

clean:
	$(RM) $(TARGET)_*
