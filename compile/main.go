// find replace string pairs in a dir
// no regex
// version 2019-01-13
// website: http://xahlee.info/golang/goland_find_replace.html

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type FRPair struct {
	dir     string // directory to search
	pkgName string // name of the package
	fs      string // find string
	rs      string // replace string
}

const (
	fnameRegex   = `\.go`
	writeToFile  = true
	doBackup     = false
	backupSuffix = "~~"
)

var dirsToSkip = []string{".git", "compile", "vendor"}
var currentPair FRPair
var pairs []FRPair
var pkgs []string

// fileList if not empty, only these are processed. Each element is a full path
var fileList = []string{}

// stringMatchAny return true if x equals any of y
func stringMatchAny(x string, y []string) bool {
	for _, v := range y {
		if x == v {
			return true
		}
	}
	return false
}

func doFile(path string) error {
	contentBytes, er := ioutil.ReadFile(path)
	if er != nil {
		panic(er)
	}
	var content = string(contentBytes)
	var changed = false
	var found = strings.Index(content, currentPair.fs)
	if found != -1 {
		content = strings.Replace(content, currentPair.fs, currentPair.rs, -1)
		changed = true
	}
	if changed {
		fmt.Printf("〘%v〙\n", path)

		if writeToFile {
			if doBackup {
				err := os.Rename(path, path+backupSuffix)
				if err != nil {
					panic(err)
				}
			}
			err2 := ioutil.WriteFile(path, []byte(content), 0644)
			if err2 != nil {
				panic("write file problem")
			}
		}
	}
	return nil
}

var pWalker = func(pathX string, infoX os.FileInfo, errX error) error {
	if errX != nil {
		fmt.Printf("error 「%v」 at a path 「%q」\n", errX, pathX)
		return errX
	}
	if infoX.IsDir() {
		if stringMatchAny(filepath.Base(pathX), dirsToSkip) {
			return filepath.SkipDir
		}
	} else {
		var x, err = regexp.MatchString(fnameRegex, filepath.Base(pathX))
		if err != nil {
			panic("stupid MatchString error 59767")
		}
		if x {
			doFile(currentPair.dir + "/" + currentPair.pkgName + ".go")
		}
	}
	return nil
}

func main() {
	_, errPath := os.Executable()
	if errPath != nil {
		panic(errPath)
	}

	os.RemoveAll("./build")
	os.RemoveAll("./src")

	run("mkdir", "./build")
	run("mkdir", "./src")

	// build yantra base package (not in a folder)

	pkgs = []string{
		"yantra",
		"exchanges",
		"optimize",
		"utils",
		"ta",
		"database",
	}

	for _, pkg := range pkgs {
		p := FRPair{
			pkgName: pkg,
			dir:     "../yantra",
			fs:      "package " + pkg,
			rs:      "package main",
		}
		// convert to
		fmt.Println("converting", p.fs, "to", p.rs)
		err := filepath.Walk(currentPair.dir, pWalker)
		if err != nil {
			fmt.Printf("error walking the path %q: %v\n", currentPair.dir, err)
		}

		os.Chdir(p.dir)
		fmt.Println("compiling", p.pkgName, "plugin")
		out := "../compile/build/" + p.pkgName + ".so"
		run("go", "build", "-buildmode", "plugin", "-o", out)

		// convert back
		fmt.Println("converting", p.rs, "to", p.fs)
		currentPair.rs = p.fs
		currentPair.fs = p.rs
		err = filepath.Walk(currentPair.dir, pWalker)

		os.Chdir("./compile")
		if err != nil {
			fmt.Printf("error walking the path %q: %v\n", currentPair.dir, err)
		}
	}
}

func run(app string, args ...string) {
	cmd := exec.Command(app, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
}

func copy(fromFile string, toFile string) {
	from, err := os.Open(fromFile)
	if err != nil {
		log.Fatal(err)
	}
	defer from.Close()

	to, err := os.OpenFile(toFile, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		log.Fatal(err)
	}
}
