package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/james-antill/tree"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var (
	// List
	I = flag.String("ignore", "", "")
	L = flag.Int("level", -1, "")
	P = flag.String("pattern", "", "")

	a = flag.Bool("all", false, "")
	d = flag.Bool("dirs-only", false, "")
	f = flag.Bool("full-path", false, "")
	l = flag.Bool("follow", false, "")
	o = flag.String("output", "", "")

	ignorecase = flag.Bool("ignore-case", false, "")
	noreport   = flag.Bool("noreport", false, "")

	// Files
	D = flag.Bool("mtime", false, "")

	g = flag.Bool("gid", false, "")
	h = flag.Bool("human", false, "")
	p = flag.Bool("protections", false, "")
	s = flag.Bool("bytes", false, "")
	u = flag.Bool("uid", false, "")

	device = flag.Bool("device", false, "")
	inodes = flag.Bool("inodes", false, "")

	// Sort
	U         = flag.Bool("U", false, "")
	v         = flag.Bool("v", false, "")
	t         = flag.Bool("t", false, "")
	c         = flag.Bool("c", false, "")
	r         = flag.Bool("r", false, "")
	dirsfirst = flag.Bool("dirsfirst", false, "")
	sort      = flag.String("sort", "", "")

	// Graphics
	C = flag.Bool("C", false, "")
	F = flag.Bool("classify", false, "")
	J = flag.Bool("nojoin", false, "")
	Q = flag.Bool("quote", false, "")

	i = flag.Bool("noindent", false, "")

	numericIDs = flag.Bool("numeric-uid-gid", false, "")
)

var usage = `Usage: tree [options...] [paths...]

Options:
    ----------------------- Listing options ----------------------
    -I --ignore          Do not list files that match the given pattern.
    -L --levels          Descend only N level dirs. deep (0=all, -1=auto (def)).
    -P --pattern         List only those files that match the pattern given.
    -a --all             All files are listed.
    -d --dirs-only       List directories only.
    -f --full-path       Print the full path prefix for each file.
    -l --follow          Follow symbolic links like directories.
    -o --output filename Output to file instead of stdout.
    --ignore-case        Ignore case when pattern matching.
    --noreport	         Turn off file/directory count at end of tree listing.

    ----------------------- File options -------------------------
    -D --mtime           Print the date of last modification change.
    -g --gid             Displays file group owner or GID number.
    -h --human           Print the size in a more human readable way.
    -p --protections     Print the protections for each file.
    -u --uid             Displays file owner or UID number.
    -s --bytes           Print the size in bytes of each file.
    --device             Print device ID number to which each file belongs.
    --inodes             Print inode number of each file.

    ---------------------- Sorting options -----------------------
    -U                   Leave files unsorted.
    -c                   Sort files by last status change time.
    -r                   Reverse the order of the sort.
    -t                   Sort files by last modification time.
    -v                   Sort files alphanumerically by version.
    --dirsfirst          List directories before files (-U disables).
    --sort X             Select sort: name,version,size,mtime,ctime.

    ---------------------- Graphics options ----------------------
    -C --color           Turn colorization on always. (def: on for terminals)
    -F --classify        Append indicator (one of */=>@|) to entries.
    -J --nojoin          Turn joining of single directories off.
    -Q --quote           Quote filenames with double quotes.
    -i --noindent        Don't print indentation lines.
    --numeric-uid-gid    Print the user and group IDs as numbers.
`

type fs struct{}

func (f *fs) Stat(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}
func (f *fs) ReadDir(path string) ([]string, error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return nil, err
	}
	return names, nil
}

func normPath(root string) (string, error) {
	ret, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root = ret

	fi, err := os.Lstat(root)
	if err != nil {
		return "", err
	}

	if (fi.Mode() & os.ModeSymlink) != 0 {
		nr, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		return nr, nil
	}

	return root, nil
}

func main() {
	// List
	flag.StringVar(I, "I", *I, "alias for --ignore")
	flag.IntVar(L, "L", *L, "alias for --level")
	flag.StringVar(P, "P", *P, "alias for --pattern")

	flag.BoolVar(a, "a", *a, "alias for --all")
	flag.BoolVar(d, "d", *d, "alias for --dirs-only")
	flag.BoolVar(f, "f", *f, "alias for --full-path")
	flag.BoolVar(l, "l", *l, "alias for --follow")
	flag.StringVar(o, "o", *o, "alias for --output")

	// Files
	flag.BoolVar(D, "D", *D, "alias for --mtime")
	flag.BoolVar(g, "g", *g, "alias for --gid")
	flag.BoolVar(h, "h", *h, "alias for --human")
	flag.BoolVar(p, "p", *p, "alias for --protections")
	flag.BoolVar(s, "s", *s, "alias for --bytes")
	flag.BoolVar(u, "u", *u, "alias for --uid")

	// Graphics
	flag.BoolVar(F, "F", *F, "alias for classify")
	flag.BoolVar(J, "J", *J, "alias for --nojoin")
	flag.BoolVar(Q, "Q", *Q, "alias for --quote")
	flag.BoolVar(i, "i", *i, "alias for --noindent")

	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var nd, nf int
	var ns int64
	var dirs = []string{"."}
	flag.Parse()
	// Make it work with leading dirs
	if args := flag.Args(); len(args) > 0 {
		dirs = args
	}
	// Output file
	var outFile = os.Stdout
	var err error
	if *o != "" {
		outFile, err = os.Create(*o)
		if err != nil {
			errAndExit(err)
		}
	} else if terminal.IsTerminal(int(os.Stdout.Fd())) {
		*C = true
	}
	defer outFile.Close()
	// Check sort-type
	if *sort != "" {
		switch *sort {
		case "version", "mtime", "ctime", "name", "size":
		default:
			msg := fmt.Sprintf("sort type '%s' not valid, should be one of: "+
				"name,version,size,mtime,ctime", *sort)
			errAndExit(errors.New(msg))
		}
	}
	// Set options
	opts := &tree.Options{
		// Required
		Fs:      new(fs),
		OutFile: outFile,
		// List
		All:        *a,
		DirsOnly:   *d,
		FullPath:   *f,
		DeepLevel:  *L,
		FollowLink: *l,
		Pattern:    *P,
		IPattern:   *I,
		IgnoreCase: *ignorecase,
		// Files
		ByteSize: *s,
		UnitSize: *h,
		FileMode: *p,
		ShowUid:  *u,
		ShowGid:  *g,
		LastMod:  *D,
		Inodes:   *inodes,
		Device:   *device,
		// Sort
		NoSort:    *U,
		ReverSort: *r,
		DirSort:   *dirsfirst,
		VerSort:   *v || *sort == "version",
		ModSort:   *t || *sort == "mtime",
		CTimeSort: *c || *sort == "ctime",
		NameSort:  *sort == "name",
		SizeSort:  *sort == "size",
		// Graphics
		NoIndent:   *i,
		Colorize:   *C,
		JoinSingle: !*J,
		Classify:   *F,
		Quotes:     *Q,
		NumericIDs: *numericIDs,
	}
	for _, dir := range dirs {
		if d, e := normPath(dir); e == nil {
			dir = d
		}
		inf := tree.New(dir)
		d, f := inf.Visit(opts)
		nd, nf = nd+d, nf+f
		nsize := tree.NodeSize(inf)
		ns += nsize
		inf.Print(opts)
	}
	// Print footer report
	if !*noreport {
		p := message.NewPrinter(language.Make(os.Getenv("LANG")))

		footer := p.Sprintf("\n%d directories", nd)
		if !opts.DirsOnly {
			footer += p.Sprintf(", %d files", nf)
		}
		showSize := opts.UnitSize || opts.ByteSize
		if showSize {
			if opts.UnitSize {
				footer += fmt.Sprintf(", %s size", tree.FormatSize(opts, ns))
			} else {
				footer += p.Sprintf(", %d size", ns)
			}
		}
		fmt.Fprintln(outFile, footer)
	}
}

func usageAndExit(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, msg)
		fmt.Fprintf(os.Stderr, "\n\n")
	}
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}

func errAndExit(err error) {
	fmt.Fprintf(os.Stderr, "tree: \"%s\"\n", err)
	os.Exit(1)
}
