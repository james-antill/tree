package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Escape = "\x1b"
const (
	Reset int = 0
	// Not used, remove.
	Bold  int = 1
	Black int = iota + 28
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
)

// Easy "port" of the default dircolors...
// Ref: https://github.com/coreutils/coreutils/blob/master/src/dircolors.hin
var cArchivesOrCompressed = []string{
	".tar",
	".tgz",
	".arc",
	".arj",
	".taz",
	".lha",
	".lz4",
	".lzh",
	".lzma",
	".tlz",
	".txz",
	".tzo",
	".t7z",
	".zip",
	".z",
	".dz",
	".gz",
	".lrz",
	".lz",
	".lzo",
	".xz",
	".zst",
	".tzst",
	".bz2",
	".bz",
	".tbz",
	".tbz2",
	".tz",
	".deb",
	".rpm",
	".jar",
	".war",
	".ear",
	".sar",
	".rar",
	".alz",
	".ace",
	".zoo",
	".cpio",
	".7z",
	".rz",
	".cab",
	".wim",
	".swm",
	".dwm",
	".esd",
}

var cImages = []string{
	".jpg",
	".jpeg",
	".mjpg",
	".mjpeg",
	".gif",
	".bmp",
	".pbm",
	".pgm",
	".ppm",
	".tga",
	".xbm",
	".xpm",
	".tif",
	".tiff",
	".png",
	".svg",
	".svgz",
	".mng",
	".pcx",
	".mov",
	".mpg",
	".mpeg",
	".m2v",
	".mkv",
	".webm",
	".webp",
	".ogm",
	".mp4",
	".m4v",
	".mp4v",
	".vob",
	".qt",
	".nuv",
	".wmv",
	".asf",
	".rm",
	".rmvb",
	".flc",
	".avi",
	".fli",
	".flv",
	".gl",
	".dl",
	".xcf",
	".xwd",
	".yuv",
	".cgm",
	".emf",

	".ogv",
	".ogx",
}

var cAudios = []string{
	".aac",
	".au",
	".flac",
	".m4a",
	".mid",
	".midi",
	".mka",
	".mp3",
	".mpc",
	".ogg",
	".ra",
	".wav",

	".oga",
	".opus",
	".spx",
	".xspf",
}

// ANSIColor
func ANSIColor(node *Node, s string) string {
	var style string
	var mode = node.Mode()
	var ext = filepath.Ext(node.Name())
	switch {
	case contains([]string{".bat", ".btm", ".cmd", ".com", ".dll", ".exe"}, ext):
		style = "1;32"
	case contains(cArchivesOrCompressed, ext):
		style = "1;31"
	case contains(cImages, ext):
		style = "1;35"
	case contains(cAudios, ext):
		style = "1;36"
	case node.IsDir() || mode&os.ModeDir != 0:
		style = "1;34"
	case mode&os.ModeNamedPipe != 0:
		style = "40;33"
	case mode&os.ModeSocket != 0:
		style = "40;1;35"
	case mode&os.ModeDevice != 0 || mode&os.ModeCharDevice != 0:
		style = "40;1;33"
	case mode&os.ModeSymlink != 0:
		if _, err := filepath.EvalSymlinks(node.path); err != nil {
			style = "40;1;31"
		} else {
			style = "1;36"
		}
	case mode&modeExecute != 0:
		style = "1;32"
	default:
		return s
	}
	return fmt.Sprintf("%s[%sm%s%s[%dm", Escape, style, s, Escape, Reset)
}

// case-insensitive contains helper
func contains(slice []string, str string) bool {
	for _, val := range slice {
		if val == strings.ToLower(str) {
			return true
		}
	}
	return false
}

// TODO: HTMLColor
