prefix ?= $(HOME)
out := $(prefix)/bin

git-rofs: main.go
	go build -o $@ $<

install: git-rofs
	install -D $< $(out)/$<
