package tree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	NoIndent bool
	Colorize bool
}

// New get path and create new node(root).
func New(path string) *Node {
	return &Node{path: path, vpaths: make(map[string]bool)}
}

// Visit all files under the given node.
func (node *Node) Visit(opts *Options) (dirs, files int) {
	// visited paths
	if path, err := filepath.Abs(node.path); err == nil {
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
	if opts.DeepLevel > 0 && opts.DeepLevel <= node.depth {
		return
	}
	names, err := opts.Fs.ReadDir(node.path)
	if err != nil {
		node.err = err
		return
	}
	node.nodes = make(Nodes, 0)
	for _, name := range names {
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
		nnode := &Node{
			path:   filepath.Join(node.path, name),
			depth:  node.depth + 1,
			vpaths: node.vpaths,
		}
		d, f := nnode.Visit(opts)
		if nnode.err == nil && !nnode.IsDir() {
			// "dirs only" option
			if opts.DirsOnly {
				continue
			}
			var rePrefix string
			if opts.IgnoreCase {
				rePrefix = "(?i)"
			}
			// Pattern matching
			if opts.Pattern != "" {
				re, err := regexp.Compile(rePrefix + opts.Pattern)
				if err == nil && !re.MatchString(name) {
					continue
				}
			}
			// IPattern matching
			if opts.IPattern != "" {
				re, err := regexp.Compile(rePrefix + opts.IPattern)
				if err == nil && re.MatchString(name) {
					continue
				}
			}
		}
		node.nodes = append(node.nodes, nnode)
		dirs, files = dirs+d, files+f
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
	case opts.DirSort:
		fn = DirSort
	case opts.VerSort:
		fn = VerSort
	case opts.SizeSort:
		fn = SizeSort
	case opts.NameSort:
		fn = NameSort
	default:
		fn = NameSort // Default should be sorted, not unsorted.
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
func (node *Node) Print(opts *Options) { node.print("", "", 0, opts) }

func dirRecursiveChildren(opts *Options, node *Node) (num int64, err error) {
	if opts.DeepLevel > 0 && node.depth >= opts.DeepLevel {
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

func dirRecursiveSize(opts *Options, node *Node) (size int64, err error) {
	if opts.DeepLevel > 0 && node.depth >= opts.DeepLevel {
		err = errors.New("Depth too high")
	}

	for _, nnode := range node.nodes {
		if nnode.err != nil {
			err = nnode.err
			continue
		}

		if !nnode.IsDir() {
			size += nnode.Size()
		} else {
			nsize, e := dirRecursiveSize(opts, nnode)
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

func chopChildren(dchildren int64) int64 {
	for i, v := range chopChildrenKey {
		if v > dchildren {
			continue
		}

		return chopChildrenVal[i]
	}

	return chopChildrenVal[len(chopChildrenVal)-1]
}

func (node *Node) print(indentc, indentn string, sofar int64, opts *Options) {
	if node.err != nil {
		err := node.err.Error()
		if msgs := strings.Split(err, ": "); len(msgs) > 1 {
			err = msgs[1]
		}
		fmt.Printf("%s [%s]\n", node.path, err)
		return
	}
	var psize int
	if !node.IsDir() {
		var props []string
		ok, inode, device, uid, gid := getStat(node)
		// inodes
		if ok && opts.Inodes {
			props = append(props, fmt.Sprintf("%d", inode))
		}
		// device
		if ok && opts.Device {
			props = append(props, fmt.Sprintf("%3d", device))
		}
		// Mode
		if opts.FileMode {
			props = append(props, node.Mode().String())
		}
		// Owner/Uid
		if ok && opts.ShowUid {
			uidStr := strconv.Itoa(int(uid))
			if u, err := user.LookupId(uidStr); err != nil {
				props = append(props, fmt.Sprintf("%-8s", uidStr))
			} else {
				props = append(props, fmt.Sprintf("%-8s", u.Username))
			}
		}
		// Gorup/Gid
		// TODO: support groupname
		if ok && opts.ShowGid {
			gidStr := strconv.Itoa(int(gid))
			props = append(props, fmt.Sprintf("%-4s", gidStr))
		}
		// Size
		if opts.ByteSize || opts.UnitSize {
			var size string
			if opts.UnitSize {
				size = fmt.Sprintf("%4s", formatBytes(node.Size()))
			} else {
				size = fmt.Sprintf("%11d", node.Size())
			}
			props = append(props, size)
		}
		// Last modification
		if opts.LastMod {
			props = append(props, node.ModTime().Format("Jan 02 15:04"))
		}
		// Print properties
		if len(props) == 1 {
			fmt.Fprintf(opts.OutFile, "%s ", strings.Join(props, " "))
		} else if len(props) > 0 {
			fmt.Fprintf(opts.OutFile, "[%s]  ", strings.Join(props, " "))
		}
	} else {
		var props []string
		// Size
		if opts.ByteSize || opts.UnitSize {
			var size string
			rsize, err := dirRecursiveSize(opts, node)
			if err != nil && rsize <= 0 {
				if opts.UnitSize {
					size = "????"
				} else {
					size = "???????????"
				}
			} else if opts.UnitSize {
				size = fmt.Sprintf("%4s", formatBytes(rsize))
			} else {
				size = fmt.Sprintf("%11d", rsize)
			}
			props = append(props, size)
		}
		// Print properties
		if len(props) > 0 {
			psize, _ = fmt.Fprintf(opts.OutFile, "%s ", strings.Join(props, " "))
		}
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
	// Print file details
	// the main idea of the print logic came from here: github.com/campoy/tools/tree
	fmt.Fprintf(opts.OutFile, "%s%s\n", indentc, name)

	children := int64(len(node.nodes))

	if opts.DeepLevel == -1 && sofar == 0 {
		sofar = chopChildren(children)
	} else if opts.DeepLevel == -1 && node.IsDir() {
		if children > sofar {
			recChildren, err := dirRecursiveChildren(opts, node)
			if err != nil && recChildren == 1 {
				fmt.Fprintf(opts.OutFile, "%*s%s%s[more file(s)]\n", psize, "", indentn, "┖┄ ")
			} else {
				fmt.Fprintf(opts.OutFile, "%*s%s%s[%d file(s)]\n", psize, "", indentn, "┖┄ ", recChildren)
			}
			return
		}

		if children > sofar {
			sofar = 1
		} else {
			sofar -= children
		}
	}

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

		nnode.print(indentc, indentn+add, sofar, opts)
	}
}
