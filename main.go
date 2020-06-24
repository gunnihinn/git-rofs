package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

    -commit HASH    Commit to check out
    -verbose        Print debug log messages
    -h, -help       Print help and exit

The argument to commit can be anything that git rev-parse can resolve
into a commit hash.

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
		if entry.Filemode == git.FilemodeBlob {
			attrs.Mode = os.FileMode(0644)
		} else if entry.Filemode == git.FilemodeBlobExecutable {
			attrs.Mode = os.FileMode(0755)
		}
		// TODO: Deal with git.FilemodeLink
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

func NewGitROFS(repo *git.Repository, commit *git.Commit) (GitROFS, error) {
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
	logger.Debugw("CreateFile")
	return fmt.Errorf("CreateFile")
}

func (fs GitROFS) CreateLink(ctx context.Context, op *fuseops.CreateLinkOp) error {
	logger.Debugw("CreateLink")
	return fmt.Errorf("CreateLink")
}

func (fs GitROFS) CreateSymlink(ctx context.Context, op *fuseops.CreateSymlinkOp) error {
	logger.Debugw("CreateSymlink")
	return fmt.Errorf("CreateSymlink")
}

func (fs GitROFS) Destroy() {
	logger.Debugw("Destroy")
}

func (fs GitROFS) Fallocate(ctx context.Context, op *fuseops.FallocateOp) error {
	logger.Debugw("Fallocate")
	return fmt.Errorf("Fallocate")
}

func (fs GitROFS) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) error {
	logger.Debugw("FlushFile", "inode", op.Inode)
	return nil
}

func (fs GitROFS) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) error {
	logger.Debugw("ForgetInode", "inode", op.Inode)
	// TODO: Something?
	return nil
}

func (fs GitROFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) error {
	logger.Debugw("GetInodeAttributes", "inode", op.Inode)

	fs.inodes.Lock()
	entry, err := fs.inodes.Get(op.Inode)
	if err != nil {
		return err
	}
	fs.inodes.Unlock()

	att, err := fs.toInodeAttributes(entry)
	if err != nil {
		logger.Error(err)
		return fuse.ENOATTR
	}

	op.Attributes = att

	return nil
}

func (fs GitROFS) GetXattr(ctx context.Context, op *fuseops.GetXattrOp) error {
	logger.Debugw("GetXattr", "inode", op.Inode, "name", op.Name)
	return fuse.ENOATTR
}

func (fs GitROFS) ListXattr(ctx context.Context, op *fuseops.ListXattrOp) error {
	logger.Debugw("ListXattr")
	return nil
}

func (fs GitROFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) error {
	logger.Debugw("MkDir")
	return fmt.Errorf("MkDir")
}

func (fs GitROFS) MkNode(ctx context.Context, op *fuseops.MkNodeOp) error {
	logger.Debugw("MkNode")
	return fmt.Errorf("MkNode")
}

func (fs GitROFS) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) error {
	logger.Debugw("LookUpInode", "parent", op.Parent, "name", op.Name)

	fs.inodes.Lock()
	id, entry, err := fs.inodes.Lookup(op.Parent, op.Name)
	fs.inodes.Unlock()

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
	logger.Debugw("OpenDir", "inode", op.Inode)
	// We can open all dirs
	return nil
}

func (fs GitROFS) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) error {
	logger.Debugw("OpenFile", "inode", op.Inode)
	// We can open all files
	return nil
}

func (fs GitROFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) error {
	logger.Debugw("ReadDir", "inode", op.Inode)

	fs.inodes.Lock()
	entry, err := fs.inodes.Get(op.Inode)
	fs.inodes.Unlock()
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
			logger.Errorw("Unexpected git object",
				"type", child.Type)
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
	logger.Debugw("ReadFile", "inode", op.Inode)

	fs.inodes.Lock()
	entry, err := fs.inodes.Get(op.Inode)
	fs.inodes.Unlock()
	if err != nil {
		return fuse.ENOENT
	}

	if entry.Type != git.ObjectBlob {
		return fuse.EINVAL
	}

	blob, err := fs.repo.LookupBlob(entry.Id)
	if err != nil {
		return fuse.EIO
	}

	r := bytes.NewReader(blob.Contents())

	op.BytesRead, err = r.ReadAt(op.Dst, op.Offset)
	if err == io.EOF {
		return nil
	}

	return err
}

func (fs GitROFS) ReadSymlink(ctx context.Context, op *fuseops.ReadSymlinkOp) error {
	logger.Debugw("ReadSymlink")
	return fmt.Errorf("ReadSymlink")
}

func (fs GitROFS) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) error {
	logger.Debugw("ReleaseDirHandle", "handle", op.Handle)
	// TODO: Something?
	return nil
}

func (fs GitROFS) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) error {
	logger.Debugw("ReleaseFileHandle", "handle", op.Handle)
	return nil
}

func (fs GitROFS) RemoveXattr(ctx context.Context, op *fuseops.RemoveXattrOp) error {
	logger.Debugw("RemoveXattr")
	return fmt.Errorf("RemoveXattr")
}

func (fs GitROFS) Rename(ctx context.Context, op *fuseops.RenameOp) error {
	logger.Debugw("Rename")
	return fmt.Errorf("Rename")
}

func (fs GitROFS) RmDir(ctx context.Context, op *fuseops.RmDirOp) error {
	logger.Debugw("RmDir")
	return fmt.Errorf("RmDir")
}

func (fs GitROFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) error {
	logger.Debugw("SetInodeAttributes")
	return fmt.Errorf("SetInodeAttributes")
}

func (fs GitROFS) SetXattr(ctx context.Context, op *fuseops.SetXattrOp) error {
	logger.Debugw("SetXattr")
	return fmt.Errorf("SetXattr")
}

func (fs GitROFS) StatFS(ctx context.Context, op *fuseops.StatFSOp) error {
	logger.Debugw("StatFS")
	return fmt.Errorf("StatFS")
}

func (fs GitROFS) SyncFile(ctx context.Context, op *fuseops.SyncFileOp) error {
	logger.Debugw("SyncFile")
	return fmt.Errorf("SyncFile")
}

func (fs GitROFS) Unlink(ctx context.Context, op *fuseops.UnlinkOp) error {
	logger.Debugw("Unlink")
	return fmt.Errorf("Unlink")
}

func (fs GitROFS) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) error {
	logger.Debugw("WriteFile")
	return fmt.Errorf("WriteFile")
}

var logger *zap.SugaredLogger

func setupLogger(debug bool) (*zap.Logger, error) {
	logcfg := zap.NewProductionConfig()
	if debug {
		logcfg.Level.SetLevel(zapcore.DebugLevel)
	}
	l, err := logcfg.Build()
	if err != nil {
		return nil, err
	}

	return l, nil
}

func main() {
	flags := struct {
		help_s  *bool
		help_l  *bool
		commit  *string
		verbose *bool
		cpuprof *string
		memprof *string
	}{
		help_s:  flag.Bool("h", false, "Print help and exit"),
		help_l:  flag.Bool("help", false, "Print help and exit"),
		commit:  flag.String("commit", "HEAD", "Commit to check out"),
		verbose: flag.Bool("verbose", false, "Debug logging"),
		cpuprof: flag.String("cpu-profile", "", "Write CPU profile to given file"),
		memprof: flag.String("mem-profile", "", "Write memory profile to given file"),
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

	l, err := setupLogger(*flags.verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't initialize logging: %s\n", err)
		os.Exit(1)
	}
	defer l.Sync()
	logger = l.Sugar()

	if *flags.cpuprof != "" {
		fh, err := os.Create(*flags.cpuprof)
		if err != nil {
			logger.Fatalw("Couldn't create CPU profile file", "error", err)
		}
		defer fh.Close()

		if err := pprof.StartCPUProfile(fh); err != nil {
			logger.Fatalw("Couldn't start CPU profile", "error", err)
		}
		defer pprof.StopCPUProfile()
	}

	// Start main program setup
	root := flag.Arg(0)
	mountPoint := flag.Arg(1)

	repo, err := git.OpenRepository(root)
	if err != nil {
		logger.Fatal(err)
	}

	obj, err := repo.RevparseSingle(*flags.commit)
	if err != nil {
		logger.Panic(err)
	}

	commit, err := obj.AsCommit()
	if err != nil {
		logger.Panic(err)
	}

	fs, err := NewGitROFS(repo, commit)
	if err != nil {
		logger.Panic(err)
	}

	logger.Infow("Creating server")
	server := fuseutil.NewFileSystemServer(fs)

	logger.Infow("Mounting filesystem")
	mfs, err := fuse.Mount(mountPoint, server, &fuse.MountConfig{})
	if err != nil {
		logger.Panic(err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		logger.Infow("Unmounting filesystem")
		if err := fuse.Unmount(mountPoint); err != nil {
			logger.Panic(err)
		}
	}()

	logger.Infow("Serving files")
	mfs.Join(context.Background())

	if *flags.memprof != "" {
		fh, err := os.Create(*flags.memprof)
		if err != nil {
			logger.Fatalw("Couldn't create memory profile file", "error", err)
		}
		defer fh.Close()

		runtime.GC()
		if err := pprof.WriteHeapProfile(fh); err != nil {
			logger.Fatalw("Couldn't write memory profile", "error", err)
		}
	}

	logger.Infow("Done")
}
