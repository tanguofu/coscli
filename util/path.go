package util

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
)

func IsCosPath(path string) bool {
	if len(path) <= 6 {
		return false
	}
	if path[:6] == "cos://" {
		return true
	} else {
		return false
	}
}

func IsDirExists(path string) bool {

	if info, err := os.Stat(path); os.IsNotExist(err) {
		return false
	} else if !info.IsDir() {
		return false
	}

	return true
}

func ParsePath(url string) (bucketName string, path string) {

	var err error
	if IsCosPath(url) {
		res := strings.SplitN(url[6:], "/", 2)
		if len(res) < 2 {
			return res[0], ""
		} else {
			return res[0], res[1]
		}
	} else {
		if url[0] == '~' {
			home, _ := homedir.Dir()
			path = home + url[1:]
			return "", path
		}

		path, err = filepath.Abs(url)
		if err != nil {
			return "", url
		}
		return "", path
	}
}
