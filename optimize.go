package algo

import (
	"context"
	"fmt"
	"log"
	// "math/rand"
	"time"

	"github.com/c-bata/goptuna"
	// "github.com/c-bata/goptuna/successivehalving"
	"github.com/c-bata/goptuna/tpe"
	"golang.org/x/sync/errgroup"

	eaopt "github.com/tantralabs/eaopt"
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

func OESOptimize(Evaluate func([]float64) float64, sigma []float64) {
	currentRunUUID = time.Now()
	var ga, err = eaopt.NewOES(1000, 10, sigma, 0.005, true, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	// Run minimization
	_, y, err := ga.Minimize(Evaluate, sigma)
	if err != nil {
		fmt.Println(err)
		return
	}
	var best = ga.GA.HallOfFame[0]
	log.Println(best)
	fmt.Printf("Found minimum of %.5f\n", y)
}

func DiffEvoOptimize(Evaluate func([]float64) float64, min, max []float64) {
	currentRunUUID = time.Now()
	var ga, err = eaopt.NewDiffEvo(400, 100, 0.5, 0.2, min, max, true, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	// Run minimization
	_, y, err := ga.Minimize(Evaluate, uint(len(min)))
	if err != nil {
		fmt.Println(err)
		return
	}
	var best = ga.GA.HallOfFame[0]
	log.Println(best)
	fmt.Printf("Found minimum of %.5f\n", y)
}