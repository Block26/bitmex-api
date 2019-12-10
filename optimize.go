package algo

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/c-bata/goptuna"
	// "github.com/c-bata/goptuna/successivehalving"
	"github.com/c-bata/goptuna/tpe"
	"golang.org/x/sync/errgroup"

	eaopt "github.com/MaxHalford/eaopt"
)

func Optimize(objective func(goptuna.Trial) (float64, error), episodes int) {
	currentRunUUID = time.Now()
	study, err := goptuna.CreateStudy(
		"optmm",
		goptuna.StudyOptionSampler(tpe.NewSampler()),
		goptuna.StudyOptionSetDirection(goptuna.StudyDirectionMaximize),
		// goptuna.StudyOptionPruner(successivehalving.NewOptunaPruner()),
		// goptuna.StudyOptionSetDirection(goptuna.StudyDirectionMinimize),
	)

	if err != nil {
		log.Fatal(err)
	}
	//Multithread
	eg, ctx := errgroup.WithContext(context.Background())
	study.WithContext(ctx)
	for i := 0; i < 12; i++ {
		eg.Go(func() error {
			return study.Optimize(objective, episodes)
		})
	}
	if err := eg.Wait(); err != nil {
		log.Fatal(err)
	}

	// Print the best evaluation value and the parameters.
	v, _ := study.GetBestValue()
	p, _ := study.GetBestParams()
	log.Printf("Best evaluation value=%f", v)
	log.Println(p)
}

func EAOptimize(Evaluate func([]float64) float64, paramsDomain []float64) {
	currentRunUUID = time.Now()
	// var spso, err = eaopt.NewDefaultSPSO()
	// var ga, err = eaopt.NewDiffEvo(40, 100, 0, 0.1, 0.5, 0.2, false, nil)
	var ga, err = eaopt.NewOES(1000, 30, 10, 0.05, false, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Fix random number generation
	ga.GA.RNG = rand.New(rand.NewSource(42))

	// Run minimization
	_, y, err := ga.Minimize(Evaluate, paramsDomain)
	if err != nil {
		fmt.Println(err)
		return
	}
	// log.Println(NormalizeParamVector(x))
	var best = ga.GA.HallOfFame[0]
	log.Println(best)
	fmt.Printf("Found minimum of %.5f\n", y)
}
