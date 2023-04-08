package mr

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

func tmpFile() (string, *os.File, error) {
	ofile, err := ioutil.TempFile(".", "rdtmp")
	if err != nil {
		return "", nil, err
	}
	tmppath, err := filepath.Abs(ofile.Name())
	return tmppath, ofile, err
}

func popLast(l *sync.Mutex, s *[]int) (bool, int) {
	l.Lock()
	defer l.Unlock()
	if len(*s) > 0 {
		lastIndex := len(*s) - 1
		r := (*s)[lastIndex]
		*s = (*s)[:lastIndex]
		return true, r
	}
	return false, 0
}
