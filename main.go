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

type coords struct {
	id   fuseops.InodeID
	name string
}

type InodeMap struct {
	*sync.Mutex
	byId         map[fuseops.InodeID]*git.TreeEntry // Could be *git.Oid?
	byParent     map[coords]fuseops.InodeID
	repo         *git.Repository
	lastIssuedID fuseops.InodeID
}

func NewInodeMap(repo *git.Repository, root *git.Commit) (*InodeMap, error) {
	im := &InodeMap{
		Mutex:        &sync.Mutex{},
		byId:         make(map[fuseops.InodeID]*git.TreeEntry),
		byParent:     make(map[coords]fuseops.InodeID),
		repo:         repo,
		lastIssuedID: fuseops.InodeID(1),
	}

	tree, err := root.Tree()
	if err != nil {
		return im, err
	}

	e := &git.TreeEntry{
		Name:     "",
		Id:       tree.Id(),
		Type:     git.ObjectTree,
		Filemode: git.FilemodeTree,
	}

	im.byId[im.lastIssuedID] = e

	return im, nil
}

// Get gets an entry by inode ID.
// Caller must lock/unlock mutex.
func (im InodeMap) Get(i fuseops.InodeID) (*git.TreeEntry, error) {
	node, ok := im.byId[i]
	if !ok {
		return nil, fmt.Errorf("No inode number %d", i)
	}

	return node, nil
}

func (fs GitROFS) toInodeAttributes(entry *git.TreeEntry) (fuseops.InodeAttributes, error) {
	now := time.Now()
	attrs := fuseops.InodeAttributes{
		// TODO: What is time?
		Atime:  now,
		Mtime:  now,
		Ctime:  now,
		Crtime: now,
		Uid:    fs.uid,
		Gid:    fs.gid,
	}

	if entry.Type == git.ObjectBlob {
		blob, err := fs.repo.LookupBlob(entry.Id)
		if err != nil {
			return attrs, err
		}
		attrs.Size = uint64(blob.Size())
		attrs.Nlink = 1
		attrs.Mode = os.FileMode(0644)
	} else if entry.Type == git.ObjectTree {
		tree, err := fs.repo.LookupTree(entry.Id)
		if err != nil {
			return attrs, err
		}
		attrs.Size = 0

		// http://teaching.idallen.com/dat2330/04f/notes/links_and_inodes.html
		attrs.Nlink = 2
		for i := uint64(0); i < tree.EntryCount(); i++ {
			child := tree.EntryByIndex(i)
			if child.Type == git.ObjectTree {
				attrs.Nlink++
			}
		}

		attrs.Mode = os.ModeDir | os.FileMode(0755)
	} else {
		return attrs, fmt.Errorf("Unexpected inode type %v", entry.Type)
	}

	return attrs, nil
}

// Lookup gets a child of a given inode by name.
// Caller must lock/unlock mutex.
func (im *InodeMap) Lookup(i fuseops.InodeID, name string) (fuseops.InodeID, *git.TreeEntry, error) {
	coord := coords{i, name}
	id, ok := im.byParent[coord]
	if ok {
		node, err := im.Get(id)
		if err != nil {
			return id, node, fuse.EIO
		}
		return id, node, nil
	}

	// Haven't seen this node before, look for it
	parent, err := im.Get(i)
	if err != nil {
		return id, nil, fuse.EIO
	}

	tree, err := im.repo.LookupTree(parent.Id)
	if err != nil {
		return id, nil, fuse.EIO
	}

	entry := tree.EntryByName(name)
	if entry == nil {
		return id, nil, fuse.ENOENT
	}

	im.lastIssuedID += 1
	im.byId[im.lastIssuedID] = entry
	im.byParent[coord] = im.lastIssuedID

	return im.lastIssuedID, entry, nil
}

type GitROFS struct {
	inodes *InodeMap
	repo   *git.Repository
	uid    uint32
	gid    uint32
}

func NewGitROFS(repo *git.Repository) (GitROFS, error) {
	fs := GitROFS{}

	user, err := user.Current()
	if err != nil {
		return fs, err
	}

	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		return fs, err
	}

	gid, err := strconv.Atoi(user.Gid)
	if err != nil {
		return fs, err
	}

	// TODO: Take reference or commit in constructor
	head, err := repo.Head()
	if err != nil {
		return fs, err
	}

	obj, err := head.Peel(git.ObjectCommit)
	if err != nil {
		return fs, err
	}

	commit, err := obj.AsCommit()
	if err != nil {
		return fs, err
	}

	ins, err := NewInodeMap(repo, commit)
	if err != nil {
		return fs, err
	}

	fs.inodes = ins
	fs.repo = repo
	fs.uid = uint32(uid)
	fs.gid = uint32(gid)

	return fs, nil
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
	log.Printf("ForgetInode: inode %d\n", op.Inode)
	// TODO: Something?
	return nil
}

func (fs GitROFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	log.Printf("GetInodeAttributes: inode %d\n", op.Inode)

	fs.inodes.Lock()
	entry, err := fs.inodes.Get(op.Inode)
	if err != nil {
		return err
	}
	fs.inodes.Unlock()

	att, err := fs.toInodeAttributes(entry)
	if err != nil {
		log.Printf("ERROR: %s\n", err)
		return fuse.ENOATTR
	}

	op.Attributes = att

	return nil
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
	log.Printf("LookUpInode: Parent %d, name %s\n", op.Parent, op.Name)

	fs.inodes.Lock()
	id, entry, err := fs.inodes.Lookup(op.Parent, op.Name)
	defer fs.inodes.Unlock()

	if err != nil {
		return err
	}

	attrs, err := fs.toInodeAttributes(entry)
	if err != nil {
		return err
	}

	op.Entry = fuseops.ChildInodeEntry{
		Child:      id,
		Attributes: attrs,
	}

	return nil
}

func (fs GitROFS) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) error {
	log.Printf("OpenDir: inode %d\n", op.Inode)
	// We can open all dirs
	return nil
}

func (fs GitROFS) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) error {
	log.Printf("OpenFile: inode %d\n", op.Inode)
	return fmt.Errorf("OpenFile")
}

func (fs GitROFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	log.Printf("ReadDir: inode %d\n", op.Inode)

	entry, err := fs.inodes.Get(op.Inode)
	if err != nil {
		return err
	}

	tree, err := fs.repo.LookupTree(entry.Id)
	if err != nil {
		return err
	}

	ec := tree.EntryCount()
	if op.Offset > fuseops.DirOffset(ec) {
		return fuse.EIO
	}

	for i := uint64(op.Offset); i < ec; i++ {
		child := tree.EntryByIndex(i)

		fs.inodes.Lock()
		id, _, err := fs.inodes.Lookup(op.Inode, child.Name)
		fs.inodes.Unlock()
		if err != nil {
			return err
		}

		var tp fuseutil.DirentType
		if child.Type == git.ObjectBlob {
			tp = fuseutil.DT_File
		} else if child.Type == git.ObjectTree {
			tp = fuseutil.DT_Directory
		} else {
			log.Printf("ERROR: Unexpected git object type %v", child.Type)
			return fuse.EIO
		}

		dirent := fuseutil.Dirent{
			Offset: fuseops.DirOffset(i) + 1, // [sic]
			Inode:  id,
			Name:   child.Name,
			Type:   tp,
		}

		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], dirent)
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
	log.Printf("ReleaseDirHandle: handle %d\n", op.Handle)
	// TODO: Something?
	return nil
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
