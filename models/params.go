package models

import "log"

type Params struct {
	store map[string]map[string]interface{}
}

func (p *Params) Add(symbol string, name string, params interface{}) {
	_, ok := p.store[symbol][name]
	if ok {
		log.Fatalln("There is already a parameter stored with that name")
	} else {
		if p.store == nil {
			p.store = make(map[string]map[string]interface{})
		}
		if p.store[symbol] == nil {
			p.store[symbol] = make(map[string]interface{})
		}
		p.store[symbol][name] = params
	}
}

func (p *Params) Update(symbol string, name string, params interface{}) interface{} {
	_, ok := p.store[symbol][name]
	if ok {
		p.store[symbol][name] = params
		return params
	}
	log.Fatalln("There is no parameter stored with that name")
	return nil
}

func (p *Params) Get(symbol string, name string) interface{} {
	_, ok := p.store[symbol][name]
	if ok {
		return p.store[symbol][name]
	}
	log.Fatalln("There is no parameter stored with that name")
	return nil
}

func (p *Params) GetAllParamsForSymbol(symbol string) map[string]interface{} {
	_, ok := p.store[symbol]
	if ok {
		return p.store[symbol]
	}
	log.Fatalln("There is no parameters stored with that symbol", symbol)
	return nil
}

func (p *Params) GetAllParams() map[string]interface{} {
	tmp := make(map[string]interface{})
	for key, _ := range p.store {
		for k, v := range p.store[key] {
			tmp[key+"-"+k] = v
		}
	}
	return tmp
}
