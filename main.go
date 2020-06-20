package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

var msgHelp string = strings.TrimSpace(`
git-rofs - Mount Git commits as read-only filesystems

USE

    git-rofs [OPTION...] MOUNTPOINT
`)

type GitROFS struct {
	fuseutil.FileSystem
}

func main() {
	flags := struct {
		help *bool
	}{
		flag.Bool("help", false, "Print help and exit"),
	}
	flag.Parse()

	if *flags.help {
		fmt.Printf("%s\n", msgHelp)
		return
	}

	if len(flag.Args()) < 1 {
		fmt.Printf("Required argument MOUNTPOINT missing.\n")
		fmt.Printf("%s\n", msgHelp)
		os.Exit(1)
	}

	mountPoint := flag.Arg(0)

	fs := GitROFS{
		&fuseutil.NotImplementedFileSystem{},
	}

	log.Printf("Creating server\n")
	server := fuseutil.NewFileSystemServer(fs)

	log.Printf("Mounting filesystem\n")
	mfs, err := fuse.Mount(mountPoint, server, &fuse.MountConfig{})
	if err != nil {
		log.Panic(err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Printf("Unmounting filesystem\n")
		if err := fuse.Unmount(mountPoint); err != nil {
			log.Panic(err)
		}
	}()

	log.Printf("Serving files\n")
	mfs.Join(context.Background())
	log.Printf("Done\n")
}
