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
	"gopkg.in/libgit2/git2go.v27"
)

var msgHelp string = strings.TrimSpace(`
git-rofs - Mount Git commits as read-only filesystems

USE

    git-rofs [OPTION...] ROOT MOUNTPOINT

ARGUMENTS

    ROOT            A Git repository
    MOUNTPOINT      A directory to mount filesystem on

OPTIONS

    -h, -help   Print help and exit
`)

type GitROFS struct {
	fuseutil.FileSystem
	*git.Repository
}

func main() {
	flags := struct {
		help_s *bool
		help_l *bool
	}{
		help_s: flag.Bool("h", false, "Print help and exit"),
		help_l: flag.Bool("help", false, "Print help and exit"),
	}
	flag.Parse()

	if *flags.help_s || *flags.help_l {
		fmt.Printf("%s\n", msgHelp)
		return
	}

	if len(flag.Args()) < 2 {
		fmt.Printf("Required arguments missing.\n")
		fmt.Printf("%s\n", msgHelp)
		os.Exit(1)
	}

	root := flag.Arg(0)
	mountPoint := flag.Arg(1)

	repo, err := git.OpenRepository(root)
	if err != nil {
		log.Panic(err)
	}

	fs := GitROFS{
		&fuseutil.NotImplementedFileSystem{},
		repo,
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
