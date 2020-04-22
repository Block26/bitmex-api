package main

import (
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/tantralabs/models"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

func main() {
	// Load config
	config := models.LoadConfig("config.json")

	// remove files when local debugging
	os.RemoveAll("./tmp")
	os.RemoveAll("./tests")
	os.Remove("./main")
	os.Remove("./ca-certificates.crt")

	// Clone Algo to local directory
	r, err := git.PlainClone("./tmp", false, &git.CloneOptions{
		URL: "https://" + config.Algo,
		Auth: &http.BasicAuth{
			Username: "abc123", // yes, this can be anything except an empty string
			Password: os.Getenv("GITHUB_TOKEN"),
		},
		Progress: os.Stdout,
	})

	if err != nil {
		log.Fatalln(err.Error())
	}

	// Get Working Tree
	w, err := r.Worktree()

	if err != nil {
		log.Fatalln("working tree", err.Error())
	}

	// Checkout Algo commit hash
	if config.Commit != "latest" {
		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(config.Commit),
		})

		if err != nil {
			log.Fatalln("checkout", err.Error())
		}
	}

	// move config to tmp folder
	copy("./config.json", "./tmp/config.json")
	// cd to tmp
	os.Chdir("./tmp")
	// download deps
	run("go", "mod", "download")
	// download gotestsum
	run("go", "get", "gotest.tools/gotestsum")
	// run go mod vendor so we can get the certs from yantra
	run("go", "mod", "vendor")
	// place the certs into the root dir
	copy("./vendor/github.com/tantralabs/yantra/ca-certificates.crt", "../ca-certificates.crt")
	// run tests
	// TODO ensure these build (even though the config.json should only have commit sha of successful builds, it doesn't hurt to double check)
	run("gotestsum", "--junitfile", "results.xml")
	// create tests dir
	run("mkdir", "../tests")
	// move results to test dir
	copy("./results.xml", "../tests/results.xml")
	// Disable CGO on linux so we can build for the scratch docker
	cmd := exec.Command("go", "build", "-a", "-installsuffix", "cgo", "-o", "main", ".")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	cmd.Env = append(cmd.Env, "GOOS=linux")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
	// Go back to parent dir
	os.Chdir("..")
	// move main file we just created from tmp to this folder
	copy("./tmp/main", "./main")
	// make main executable
	run("chmod", "+x", "main")
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
