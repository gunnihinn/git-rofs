run: go-rofs
	./go-rofs . /tmp/nofs

go-rofs: main.go
	go build -o $@ $<
