CC = go build
FMT = go fmt
TARGET = cull_the_devs

default: darwin64 linux64
lambda: linux64 lambda_zip

darwin64:
	make fmt
	GOOS=darwin GOARCH=amd64 $(CC) -o $(TARGET)_darwin64 $(TARGET).go

linux64:
	make fmt
	GOOS=linux GOARCH=amd64 $(CC) -o $(TARGET)_linux64 $(TARGET).go

lambda_zip:
	zip -r $(TARGET).zip $(TARGET)_linux64

fmt:
	$(FMT) $(TARGET).go

clean:
	$(RM) $(TARGET)_* $(TARGET).zip
