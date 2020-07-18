//+build linux openbsd dragonfly android solaris

package tree

import (
	"syscall"
)

func CTimeSort(nf1, nf2 *Node) bool {
	f1 := nf1.FileInfo
	f2 := nf2.FileInfo
	s1, ok1 := f1.Sys().(*syscall.Stat_t)
	s2, ok2 := f2.Sys().(*syscall.Stat_t)
	// If this type of node isn't an os node then revert to ModSort
	if !ok1 || !ok2 {
		return ModSort(nf1, nf2)
	}
	return s1.Ctim.Sec < s2.Ctim.Sec
}
