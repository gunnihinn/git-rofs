run: git-rofs
	./git-rofs . /tmp/nofs

debug: main.go
	dlv debug $< -- . /tmp/nofs
	sudo umount /tmp/nofs

git-rofs: main.go
	go build -o $@ $<
