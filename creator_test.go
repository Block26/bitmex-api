package yantra

import (
	"os"
	"testing"

	"github.com/tantralabs/yantra/utils"
)

func TestModuleCreator(t *testing.T) {
	testDir := "../tmp-yantra-mod-test"
	unitTestFile := "results.xml"
	unitTestDir := "tests"

	utils.Run("rm", "-rf", testDir)
	// Make dir for
	utils.Run("cp", "-R", "creator/create/module_template", testDir)
	os.Chdir(testDir)
	// run unit tests
	utils.Run("gotestsum", "--junitfile", unitTestFile)
	// create tests dir
	utils.Run("mkdir", unitTestDir)
	// move results to test dirv
	utils.Copy("./"+unitTestFile, unitTestDir+"/"+unitTestFile)
	// swap back to yantra
	os.Chdir("../yantra")
}

func TestAlgoCreator(t *testing.T) {
	testDir := "../tmp-yantra-algo-test"
	unitTestFile := "results.xml"
	unitTestDir := "tests"

	utils.Run("rm", "-rf", testDir)
	// Make dir for
	utils.Run("cp", "-R", "creator/create/algo_template", testDir)
	os.Chdir(testDir)
	// run unit tests
	utils.Run("gotestsum", "--junitfile", unitTestFile)
	// create tests dir
	utils.Run("mkdir", unitTestDir)
	// move results to test dirv
	utils.Copy("./"+unitTestFile, unitTestDir+"/"+unitTestFile)
	// swap back to yantra
	os.Chdir("../yantra")
}
