package yantra

import (
	"os"

	"github.com/tantralabs/yantra/utils"
)

func ModuleTest() {
	utils.Copy("creator/create/module_template", "../tmp-yantra-test")
	os.Chdir("../tmp-yantra-test")
}
