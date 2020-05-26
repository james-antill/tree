package tree

import (
	"errors"
	"fmt"
	"golang.org/x/sync/semaphore"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Node represent some node in the tree
// contains FileInfo, and its childs
type Node struct {
	os.FileInfo
	path   string
	depth  int
	err    error
	nodes  Nodes
	sorted bool
	vpaths map[string]bool
}

// List of nodes
type Nodes []*Node

// To use this package programmatically, you must implement this
// interface.
// For example: PTAL on 'cmd/tree/tree.go'
type Fs interface {
	Stat(path string) (os.FileInfo, error)
	ReadDir(path string) ([]string, error)
}

// Options store the configuration for specific tree.
// Note, that 'Fs', and 'OutFile' are required (OutFile can be os.Stdout).
type Options struct {
	Fs      Fs
	OutFile io.Writer
	// List
	All        bool
	DirsOnly   bool
	FullPath   bool
	IgnoreCase bool
	FollowLink bool
	DeepLevel  int
	Pattern    string
	IPattern   string
	// File
	ByteSize bool
	UnitSize bool
	FileMode bool
	ShowUid  bool
	ShowGid  bool
	LastMod  bool
	Quotes   bool
	Inodes   bool
	Device   bool
	// Sort
	NoSort    bool
	VerSort   bool
	ModSort   bool
	DirSort   bool
	NameSort  bool
	SizeSort  bool
	CTimeSort bool
	ReverSort bool
	// Graphics
	NoIndent   bool
	Colorize   bool
	JoinSingle bool

	wg  sync.WaitGroup
	sem *semaphore.Weighted
	res chan workerResult
}

// workerResult for go-ness
type workerResult struct {
	p *Node
	n *Node
	d int
	f int
}

// New get path and create new node(root).
func New(path string) *Node {
	return &Node{path: path, vpaths: make(map[string]bool)}
}

func newSubNode(opts *Options, node *Node, name string) (nnode *Node, dirs, files int) {
	nnode = &Node{
		path:   filepath.Join(node.path, name),
		depth:  node.depth + 1,
		vpaths: node.vpaths,
	}
	d, f := nnode.Visit(opts)
	if nnode.err == nil && !nnode.IsDir() {
		// "dirs only" option
		if opts.DirsOnly {
			return nil, 0, 0
		}
		var rePrefix string
		if opts.IgnoreCase {
			rePrefix = "(?i)"
		}
		// Pattern matching
		if opts.Pattern != "" {
			re, err := regexp.Compile(rePrefix + opts.Pattern)
			if err == nil && !re.MatchString(name) {
				return nil, 0, 0
			}
		}
		// IPattern matching
		if opts.IPattern != "" {
			re, err := regexp.Compile(rePrefix + opts.IPattern)
			if err == nil && re.MatchString(name) {
				return nil, 0, 0
			}
		}
	}

	return nnode, d, f
}

const semWeight = 64
const rootProc = true

// Visit all files under the given node.
func (node *Node) Visit(opts *Options) (dirs, files int) {
	goProcs := !opts.FollowLink && (semWeight > 0)

	// visited paths
	if !opts.FollowLink {
		node.vpaths = nil
	} else if path, err := filepath.Abs(node.path); err == nil {
		path = filepath.Clean(path)
		node.vpaths[path] = true
	}
	// stat
	fi, err := opts.Fs.Stat(node.path)
	if err != nil {
		node.err = err
		return
	}
	node.FileInfo = fi
	if !fi.IsDir() {
		return 0, 1
	}
	// increase dirs only if it's a dir, but not the root.
	if node.depth != 0 {
		dirs++
	}
	// DeepLevel option
	showSize := opts.UnitSize || opts.ByteSize
	if !showSize && (opts.DeepLevel > 0 && opts.DeepLevel <= node.depth) {
		return
	}
	names, err := opts.Fs.ReadDir(node.path)
	if err != nil {
		node.err = err
		return
	}
	node.nodes = make(Nodes, 0)
	var rwg sync.WaitGroup
	var fin chan workerResult
	if goProcs && node.depth == 0 {
		opts.sem = semaphore.NewWeighted(semWeight)
		opts.res = make(chan workerResult, semWeight)
		rwg.Add(1)
		fin = make(chan workerResult)
		go func() {
			defer rwg.Done()
			defer close(fin)
			mdirs := 0
			mfiles := 0
			for val := range opts.res {
				val.p.nodes = append(val.p.nodes, val.n)
				mdirs, mfiles = mdirs+val.d, mfiles+val.f
			}
			fin <- workerResult{nil, node, mdirs, mfiles}
		}()
	}
	for i := range names {
		name := names[i]
		// "all" option
		if !opts.All && strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(name, "~") {
			continue
		}
		if strings.HasSuffix(name, ".bak") {
			continue
		}
		if strings.HasSuffix(name, ".swp") && false {
			continue
		}
		if goProcs && (rootProc || node.depth != 0) {
			if opts.sem.TryAcquire(2) {
				opts.wg.Add(1)
				go func() {
					defer opts.wg.Done()
					defer opts.sem.Release(2)
					nnode, d, f := newSubNode(opts, node, name)
					if nnode == nil {
						return
					}
					opts.res <- workerResult{node, nnode, d, f}
				}()
				continue
			}
		}
		nnode, d, f := newSubNode(opts, node, name)
		if nnode == nil {
			continue
		}
		if goProcs && (rootProc || node.depth != 0) {
			opts.res <- workerResult{node, nnode, d, f}
			continue
		}
		node.nodes = append(node.nodes, nnode)
		dirs, files = dirs+d, files+f
	}
	if goProcs && node.depth == 0 {
		opts.wg.Wait()
		close(opts.res)
		val := <-fin
		dirs += val.d
		files += val.f
		rwg.Wait()
	}
	return
}

func (node *Node) sortedNodes(opts *Options) Nodes {
	if !node.sorted {
		node.sort(opts)
		node.sorted = true
	}

	return node.nodes
}

func (node *Node) sort(opts *Options) {
	var fn SortFunc
	switch {
	case opts.NoSort:
		return
	case opts.ModSort:
		fn = ModSort
	case opts.CTimeSort:
		fn = CTimeSort
	case opts.VerSort:
		fn = VerSort
	case opts.SizeSort:
		fn = SizeSort
	case opts.NameSort:
		fn = NameSort
	default:
		fn = NameSort // Default should be sorted, not unsorted.
	}
	if opts.DirSort {
		nxt := fn
		fn = func(f1, f2 os.FileInfo) bool {
			return DirSort(f1, f2, nxt)
		}
	}
	if fn != nil {
		if opts.ReverSort {
			sort.Sort(sort.Reverse(ByFunc{node.nodes, fn}))
		} else {
			sort.Sort(ByFunc{node.nodes, fn})
		}
	}
}

// Print nodes based on the given configuration.
func (node *Node) Print(opts *Options) { node.print(opts, "", "", 0, nil) }

// dirDirectChildren give the direct dirs. and files for a directory
func dirDirectChildren(node *Node) (int64, int64) {
	var D int64
	var F int64
	for _, nnode := range node.nodes {
		if nnode.IsDir() {
			D++
		} else {
			F++
		}
	}
	return D, F
}

// dirDirectChildren1 give the number of direct children as a single number,
// where dirs. are counted as two because of the [files(s)] subtree.
func dirDirectChildren1(node *Node) int64 {
	D, F := dirDirectChildren(node)
	D *= 2
	return D + F
}

// dirNextLevelCutoff takes a cutoff value and returns what the limit to show
// for the next should be to be under that cutoff.
func dirNextLevelCutoff(opts *Options, node *Node, cutoff int64) int64 {
	// We could go a couple lower as we never use used[0] or used[1].
	used := make([]int64, cutoff+1)

used_loop:
	for _, nnode := range node.nodes {
		// First do the github thing...
		if opts.JoinSingle {
			for len(nnode.nodes) <= 1 {
				if len(nnode.nodes) < 1 {
					continue used_loop
				}
				nnode = nnode.nodes[0]
			}
		} else if len(nnode.nodes) <= 1 {
			continue
		}
		if !nnode.IsDir() {
			continue
		}

		children := dirDirectChildren1(nnode)
		if children >= int64(len(used)) {
			continue
		}
		used[children] += children
	}

	var tot int64
	for i := range used {
		tot += used[i]
		if tot > cutoff {
			return int64(i - 1)
		}
	}

	return cutoff
}

func dirRecursiveChildren(opts *Options, node *Node) (num int64, err error) {
	// Always called with showSize == true atm.
	showSize := opts.UnitSize || opts.ByteSize
	if !showSize && opts.DeepLevel > 0 && node.depth >= opts.DeepLevel {
		err = errors.New("Depth too high")
		return 1, err
	}

	num = int64(len(node.nodes))
	for _, nnode := range node.nodes {
		if nnode.err != nil {
			err = nnode.err
			continue
		}

		if !nnode.IsDir() {
			continue
		}

		nnum, e := dirRecursiveChildren(opts, nnode)
		if e != nil {
			err = e
		}
		num += nnum
	}

	return num, err
}

// DirRecursiveSize returns the size of the directory, as the total of all
// child nodes.
func DirRecursiveSize(opts *Options, node *Node) (size int64, err error) {
	for _, nnode := range node.nodes {
		if nnode.err != nil {
			err = nnode.err
			continue
		}

		if !nnode.IsDir() {
			size += nnode.Size()
		} else {
			nsize, e := DirRecursiveSize(opts, nnode)
			size += nsize
			if e != nil {
				err = e
			}
		}
	}
	return
}

const alwaysShowChildren = 2 // Always show this number of entries...
// Take the direct children as Key and convert to next children as Val
var chopChildrenKey = [...]int64{6_400, 3_200, 1_600, 800, 400, 200, 100,
	50, 24, 12, 8, 4}

// 24 is a normal terminal screen size... 8 for low numbers of direct children
// Because is you have a dir. with just 2 dirs. in it show more.
var chopChildrenVal = [...]int64{8 * 24, 6 * 24, 5 * 24, 4 * 24, 3 * 24, 2 * 24, 24,
	18, 12, 6, 4, 4, 8}

// Roughly allow upto 50% growth at the next level of the tree
func chopChildren(dchildren int64) int64 {
	for i, v := range chopChildrenKey {
		if v > dchildren {
			continue
		}

		return chopChildrenVal[i]
	}

	return chopChildrenVal[len(chopChildrenVal)-1]
}

// joinSingleNodes combine output like in github so a single file in a dir.
// becomes dir/file instead.
func joinSingleNodes(opts *Options, node *Node, name string) (*Node, string) {

	if !opts.JoinSingle {
		return node, name
	}

	if len(node.nodes) != 1 {
		return node, name
	}

	if opts.Inodes {
		return node, name
	}
	if opts.Device {
		return node, name
	}
	if opts.FileMode {
		return node, name
	}
	if opts.ShowUid {
		return node, name
	}
	if opts.ShowGid {
		return node, name
	}
	if opts.LastMod {
		return node, name
	}
	// Showing size is fine, because it's just an empty dir.
	if opts.FullPath {
		return node, name
	}
	nxt := node.nodes[0]

	nxtName := nxt.Name()
	// Quotes
	if opts.Quotes {
		nxtName = fmt.Sprintf("\"%s\"", nxtName)
	}
	// Colorize
	if opts.Colorize {
		nxtName = ANSIColor(nxt, nxtName)
	}
	name = filepath.Join(name, nxtName)
	return joinSingleNodes(opts, nxt, name)
}

// FormatSize as a string
func FormatSize(opts *Options, size int64) string {
	if opts.UnitSize {
		return fmt.Sprintf("%4s", formatBytes(size))
	}
	return fmt.Sprintf("%11d", size)
}

type maxTreeValues struct {
	mIno int
	mDev int
	mUid int
	mGid int
}

// numLen is a quick hack to do math.Log10(num) + 1
func numLen(num uint64) int {
	ret := 0
	for num > 0 {
		ret++
		num /= 10
	}
	return ret
}

// uidConvert takes a uid and returns the name
func uidConvert(uid uint64) string {
	uidStr := strconv.Itoa(int(uid))
	if u, err := user.LookupId(uidStr); err != nil {
		return uidStr
	} else {
		return u.Username
	}
}

// gidConvert takes a gid and returns the name
func gidConvert(gid uint64) string {
	gidStr := strconv.Itoa(int(gid))
	if g, err := user.LookupGroupId(gidStr); err != nil {
		return gidStr
	} else {
		return g.Name
	}
}

// setupMaxValues walk the entire tree and get the max values. We currently
// walk the nodes even if we don't print them ... but eh.
func (node *Node) setupMaxValues(opts *Options, maxvals *maxTreeValues) {
	ok, inode, device, uid, gid := getStat(node)
	if !ok {
		return
	}

	if opts.Inodes {
		nino := numLen(inode)
		if nino > maxvals.mIno {
			maxvals.mIno = nino
		}
	}

	if opts.Device {
		ndev := numLen(device)
		if ndev > maxvals.mDev {
			maxvals.mDev = ndev
		}
	}

	if opts.ShowUid {
		nuid := len(uidConvert(uid))
		if nuid > maxvals.mUid {
			maxvals.mUid = nuid
		}
	}

	if opts.ShowGid {
		ngid := len(gidConvert(gid))
		if ngid > maxvals.mGid {
			maxvals.mGid = ngid
		}
	}

	for _, nnode := range node.nodes {
		nnode.setupMaxValues(opts, maxvals)
	}
}

func (node *Node) print(opts *Options, indentc, indentn string,
	cutoff int64, maxvals *maxTreeValues) {
	if node.err != nil {
		err := node.err.Error()
		if msgs := strings.Split(err, ": "); len(msgs) > 1 {
			err = msgs[1]
		}
		fmt.Printf("%s [%s]\n", node.path, err)
		return
	}

	if maxvals == nil {
		maxvals = &maxTreeValues{}
		node.setupMaxValues(opts, maxvals)
	}

	var props []string
	ok, inode, device, uid, gid := getStat(node)
	// inodes
	if ok && opts.Inodes {
		props = append(props, fmt.Sprintf("%*d", maxvals.mIno, inode))
	}
	// device
	if ok && opts.Device {
		props = append(props, fmt.Sprintf("%*d", maxvals.mDev, device))
	}
	// Mode
	if opts.FileMode {
		props = append(props, node.Mode().String())
	}
	// Owner/Uid
	if ok && opts.ShowUid {
		uidStr := strconv.Itoa(int(uid))
		if u, err := user.LookupId(uidStr); err != nil {
			props = append(props, fmt.Sprintf("%-*s", maxvals.mUid, uidStr))
		} else {
			props = append(props, fmt.Sprintf("%-*s", maxvals.mUid, u.Username))
		}
	}
	// Group/Gid
	if ok && opts.ShowGid {
		gidStr := strconv.Itoa(int(gid))
		if g, err := user.LookupGroupId(gidStr); err != nil {
			props = append(props, fmt.Sprintf("%-*s", maxvals.mGid, gidStr))
		} else {
			props = append(props, fmt.Sprintf("%-*s", maxvals.mGid, g.Name))
		}
	}
	// Size
	if !node.IsDir() {
		if opts.ByteSize || opts.UnitSize {
			props = append(props, FormatSize(opts, node.Size()))
		}
	} else {
		if opts.ByteSize || opts.UnitSize {
			var size string

			rsize, err := DirRecursiveSize(opts, node)

			if err != nil && rsize <= 0 {
				if opts.UnitSize {
					size = "????"
				} else {
					size = "???????????"
				}
			} else {
				size = FormatSize(opts, rsize)
			}
			props = append(props, size)
		}
	}
	// Last modification
	if opts.LastMod {
		props = append(props, node.ModTime().Format("2006-01-02 15:04"))
	}
	// Print properties
	var psize int
	if len(props) == 1 {
		psize, _ = fmt.Fprintf(opts.OutFile, "%s ", strings.Join(props, " "))
	} else if len(props) > 0 {
		psize, _ = fmt.Fprintf(opts.OutFile, "[%s] ", strings.Join(props, " "))
	}
	// name/path
	var name string
	if node.depth == 0 || opts.FullPath {
		name = node.path
	} else {
		name = node.Name()
	}

	// Quotes
	if opts.Quotes {
		name = fmt.Sprintf("\"%s\"", name)
	}
	// Colorize
	if opts.Colorize {
		name = ANSIColor(node, name)
	}
	// Do the github thing...
	node, name = joinSingleNodes(opts, node, name)

	// IsSymlink
	if node.Mode()&os.ModeSymlink == os.ModeSymlink {
		vtarget, err := os.Readlink(node.path)
		if err != nil {
			vtarget = node.path
		}
		targetPath, err := filepath.EvalSymlinks(node.path)
		if err != nil {
			targetPath = vtarget
		}
		fi, err := opts.Fs.Stat(targetPath)
		if opts.Colorize && fi != nil {
			vtarget = ANSIColor(&Node{FileInfo: fi, path: vtarget}, vtarget)
		}
		name = fmt.Sprintf("%s -> %s", name, vtarget)
		// Follow symbolic links like directories
		if opts.FollowLink {
			path, err := filepath.Abs(targetPath)
			if err == nil && fi != nil && fi.IsDir() {
				if _, ok := node.vpaths[filepath.Clean(path)]; !ok {
					inf := &Node{FileInfo: fi, path: targetPath}
					inf.vpaths = node.vpaths
					inf.Visit(opts)
					node.nodes = inf.nodes
				} else {
					name += " [recursive, not followed]"
				}
			}
		}
	}
	fmt.Fprintf(opts.OutFile, "%s%s\n", indentc, name)

	deepLevel := opts.DeepLevel
	if deepLevel > 0 && node.depth >= deepLevel {
		// This should only be true when viewing UnitSize/ByteSize data.
		// We could just return, and look like normal. But we have the data so
		// might as well show the children too like dynamic leveling.
		deepLevel = -1
		cutoff = 1
		// But only if Level > 1, otherwise it can be a bit too spammy.
		if opts.DeepLevel == 1 {
			return
		}
	}

	// Dynamic leveling, show something but don't spam large trees.
	if deepLevel == -1 && cutoff == 0 {
		children := dirDirectChildren1(node)
		choped := chopChildren(children)
		cutoff = dirNextLevelCutoff(opts, node, choped)
		// fmt.Println("JDBG:", children, choped, cutoff)
	} else if deepLevel == -1 && node.IsDir() {
		children := dirDirectChildren1(node)
		if children > cutoff || opts.DeepLevel != -1 {
			recChildren, _ := dirRecursiveChildren(opts, node)
			p := message.NewPrinter(language.Make(os.Getenv("LANG")))
			p.Fprintf(opts.OutFile, "%*s%s%s[%d file(s)]\n", psize, "", indentn, "┖┄ ", recChildren)
			return
		}

		if children >= cutoff {
			cutoff = 1
		} else {
			cutoff -= children
		}
	}

	// Print tree structure
	// the main idea of the print logic came from here: github.com/campoy/tools/tree
	add := "┃ "
	for i, nnode := range node.sortedNodes(opts) {
		if opts.NoIndent {
			add = ""
		} else {
			if i == len(node.nodes)-1 {
				indentc = indentn + "┗━ "
				add = "  "
			} else {
				indentc = indentn + "┣━ "
			}
		}

		nnode.print(opts, indentc, indentn+add, cutoff, maxvals)
	}
}
