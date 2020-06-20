package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

type GitROFS struct {
	fuseutil.FileSystem
}

func main() {
	mountPoint := "/tmp/nofs"

	fs := GitROFS{
		&fuseutil.NotImplementedFileSystem{},
	}

	fmt.Println("Creating server")
	server := fuseutil.NewFileSystemServer(fs)

	fmt.Println("Mounting filesystem")
	mfs, err := fuse.Mount(mountPoint, server, &fuse.MountConfig{})
	if err != nil {
		panic(err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Println("Unmounting filesystem")
		if err := fuse.Unmount(mountPoint); err != nil {
			panic(err)
		}
	}()

	fmt.Println("Serving files")
	mfs.Join(context.Background())
	fmt.Println("Done")
}
