package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
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

type InodeCoords struct {
	parent fuseops.InodeID
	name   string
}

type Inode struct {
	node fuseops.ChildInodeEntry
}

type InodeMap struct {
	*sync.Mutex
	nodes        map[InodeCoords]Inode
	lastIssuedID fuseops.InodeID
}

func NewInodeMap() InodeMap {
	im := InodeMap{
		Mutex:        &sync.Mutex{},
		nodes:        make(map[InodeCoords]Inode),
		lastIssuedID: fuseops.InodeID(1),
	}

	return im
}

type HandleMaker struct {
	*sync.Mutex // Could be an atomic but whatever
	lastIssued  fuseops.HandleID
}

func NewHandleMaker() HandleMaker {
	return HandleMaker{
		Mutex:      &sync.Mutex{},
		lastIssued: fuseops.HandleID(0),
	}
}

func (hm HandleMaker) Issue() fuseops.HandleID {
	hm.Lock()
	defer hm.Unlock()
	hm.lastIssued++
	return hm.lastIssued
}

type GitROFS struct {
	inodes  InodeMap
	handles HandleMaker
	repo    *git.Repository
	uid     uint32
	gid     uint32
}

func NewGitROFS(repo *git.Repository) (GitROFS, error) {
	user, err := user.Current()
	if err != nil {
		return GitROFS{}, err
	}

	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		return GitROFS{}, err
	}

	gid, err := strconv.Atoi(user.Gid)
	if err != nil {
		return GitROFS{}, err
	}

	return GitROFS{
		inodes:  NewInodeMap(),
		handles: NewHandleMaker(),
		repo:    repo,
		uid:     uint32(uid),
		gid:     uint32(gid),
	}, nil
}

func (fs GitROFS) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) error {
	log.Printf("CreateFile\n")
	return fmt.Errorf("CreateFile")
}

func (fs GitROFS) CreateLink(ctx context.Context, op *fuseops.CreateLinkOp) error {
	log.Printf("CreateLink\n")
	return fmt.Errorf("CreateLink")
}

func (fs GitROFS) CreateSymlink(ctx context.Context, op *fuseops.CreateSymlinkOp) error {
	log.Printf("CreateSymlink\n")
	return fmt.Errorf("CreateSymlink")
}

func (fs GitROFS) Destroy() {
	log.Printf("Destroy\n")
}

func (fs GitROFS) Fallocate(ctx context.Context, op *fuseops.FallocateOp) error {
	log.Printf("Fallocate\n")
	return fmt.Errorf("Fallocate")
}

func (fs GitROFS) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) error {
	log.Printf("FlushFile\n")
	return fmt.Errorf("FlushFile")
}

func (fs GitROFS) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) error {
	log.Printf("ForgetInode\n")
	return fmt.Errorf("ForgetInode")
}

func (fs GitROFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	log.Printf("GetInodeAttributes: Inode %d\n", op.Inode)
	if op.Inode == fuseops.RootInodeID {
        // TODO: fn : git object -> fuseops.InodeAttributes
		op.Attributes = fuseops.InodeAttributes{
			Size:  0,
			Nlink: 1,
			Mode:  0555 | os.ModeDir,

			Atime:  time.Now(),
			Mtime:  time.Now(),
			Ctime:  time.Now(),
			Crtime: time.Now(),

			Uid: fs.uid,
			Gid: fs.gid,
		}
		op.AttributesExpiration = time.Now().Add(time.Duration(24) * time.Hour)

		return nil
	}
	return fmt.Errorf("GetInodeAttributes")
}

func (fs GitROFS) GetXattr(ctx context.Context, op *fuseops.GetXattrOp) error {
	log.Printf("GetXattr\n")
	return fmt.Errorf("GetXattr")
}

func (fs GitROFS) ListXattr(ctx context.Context, op *fuseops.ListXattrOp) error {
	log.Printf("ListXattr\n")
	return fmt.Errorf("ListXattr")
}

func (fs GitROFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) error {
	log.Printf("MkDir\n")
	return fmt.Errorf("MkDir")
}

func (fs GitROFS) MkNode(ctx context.Context, op *fuseops.MkNodeOp) error {
	log.Printf("MkNode\n")
	return fmt.Errorf("MkNode")
}

func (fs GitROFS) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) error {
	log.Printf("LookUpInode\n")
	log.Printf(".. Parent %d\n", op.Parent)
	log.Printf(".. Name %s\n", op.Name)

	fs.inodes.Lock()
	defer fs.inodes.Unlock()

	coords := InodeCoords{
		parent: op.Parent,
		name:   op.Name,
	}

	var n Inode
	var ok bool
	if n, ok = fs.inodes.nodes[coords]; !ok {
		fs.inodes.lastIssuedID++

		// TODO: Look up file/dir info in git repo
		n = Inode{
			node: fuseops.ChildInodeEntry{
				Child: fs.inodes.lastIssuedID,
				Attributes: fuseops.InodeAttributes{
					Size:  0,
					Nlink: 0,
					Mode:  os.FileMode(0444),

					Atime:  time.Now(),
					Mtime:  time.Now(),
					Ctime:  time.Now(),
					Crtime: time.Now(),

					Uid: fs.uid,
					Gid: fs.gid,
				},
				AttributesExpiration: time.Now().Add(time.Duration(24) * time.Hour),
				EntryExpiration:      time.Now().Add(time.Duration(24) * time.Hour),
			},
		}
		fs.inodes.nodes[coords] = n
	}

	op.Entry = n.node

	return nil
}

func (fs GitROFS) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) error {
	log.Printf("OpenDir\n")
	log.Printf("Inode %d\n", op.Inode)
	op.Handle = fs.handles.Issue()
	// We can open all dirs
	return nil
}

func (fs GitROFS) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) error {
	log.Printf("OpenFile\n")
	return fmt.Errorf("OpenFile")
}

func (fs GitROFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	log.Printf("ReadDir\n")
	log.Printf(".. inode %d\n", op.Inode)

	entries := []fuseutil.Dirent{
		fuseutil.Dirent{
			Inode:  fuseops.InodeID(2),
			Name:   "file!",
			Type:   fuseutil.DT_File,
		},
	}

	for _, entry := range entries {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], entry)
		if n == 0 {
			break
		}

		op.BytesRead += n
	}

	return nil
}

func (fs GitROFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) error {
	log.Printf("ReadFile\n")
	return fmt.Errorf("ReadFile")
}

func (fs GitROFS) ReadSymlink(ctx context.Context, op *fuseops.ReadSymlinkOp) error {
	log.Printf("ReadSymlink\n")
	return fmt.Errorf("ReadSymlink")
}

func (fs GitROFS) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) error {
	log.Printf("ReleaseDirHandle\n")
	return fmt.Errorf("ReleaseDirHandle")
}

func (fs GitROFS) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) error {
	log.Printf("ReleaseFileHandle\n")
	return fmt.Errorf("ReleaseFileHandle")
}

func (fs GitROFS) RemoveXattr(ctx context.Context, op *fuseops.RemoveXattrOp) error {
	log.Printf("RemoveXattr\n")
	return fmt.Errorf("RemoveXattr")
}

func (fs GitROFS) Rename(ctx context.Context, op *fuseops.RenameOp) error {
	log.Printf("Rename\n")
	return fmt.Errorf("Rename")
}

func (fs GitROFS) RmDir(ctx context.Context, op *fuseops.RmDirOp) error {
	log.Printf("RmDir\n")
	return fmt.Errorf("RmDir")
}

func (fs GitROFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) error {
	log.Printf("SetInodeAttributes\n")
	return fmt.Errorf("SetInodeAttributes")
}

func (fs GitROFS) SetXattr(ctx context.Context, op *fuseops.SetXattrOp) error {
	log.Printf("SetXattr\n")
	return fmt.Errorf("SetXattr")
}

func (fs GitROFS) StatFS(ctx context.Context, op *fuseops.StatFSOp) error {
	log.Printf("StatFS\n")
	return fmt.Errorf("StatFS")
}

func (fs GitROFS) SyncFile(ctx context.Context, op *fuseops.SyncFileOp) error {
	log.Printf("SyncFile\n")
	return fmt.Errorf("SyncFile")
}

func (fs GitROFS) Unlink(ctx context.Context, op *fuseops.UnlinkOp) error {
	log.Printf("Unlink\n")
	return fmt.Errorf("Unlink")
}

func (fs GitROFS) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) error {
	log.Printf("WriteFile\n")
	return fmt.Errorf("WriteFile")
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

	fs, err := NewGitROFS(repo)
	if err != nil {
		log.Panic(err)
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
