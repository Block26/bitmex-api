package optimize

import (
	"fmt"
	"log"
	"math/rand"

	eaopt "github.com/tantralabs/eaopt"
	"github.com/tantralabs/models"
)

func OESOptimize(Evaluate func([]float64) float64, sigma []float64) {
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
	var ga, err = eaopt.NewDiffEvo(400, 100, 0.5, 0.2, min, max, true, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	ga.GA.RNG = rand.New(rand.NewSource(13))
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

func ConstrainSearchParameters(searchParameters map[string]models.SearchParameter, x []float64) (sp map[string]models.SearchParameter) {
	sp = make(map[string]models.SearchParameter)
	i := 0
	for key := range searchParameters {
		sp[key] = searchParameters[key].SetValue(x[i])
		i++
	}
	return sp
}

func GetMinMaxSearchDomain(searchParameters map[string]models.SearchParameter) (min []float64, max []float64) {
	for i := range searchParameters {
		min = append(min, searchParameters[i].GetMin())
		max = append(max, searchParameters[i].GetMax())
	}
	return
}
