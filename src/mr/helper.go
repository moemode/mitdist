package mr

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

func tmpFile() (string, *os.File, error) {
	ofile, err := ioutil.TempFile(".", "rdtmp")
	if err != nil {
		return "", nil, err
	}
	tmppath, err := filepath.Abs(ofile.Name())
	return tmppath, ofile, err
}
