package models

import (
	"github.com/tantralabs/logger"
)

// The Params struct is a generic model that can map an module's parameters to the parameter names and values.
type Params struct {
	store map[string]map[string]interface{} // Maps market symbol to map of parameter names to parameter values.
}

// Add a parameter given a market symbol, parameter name, and parameter values.
// Throw an error if the parameter already exists.
func (p *Params) Add(symbol string, name string, params interface{}) {
	_, ok := p.store[symbol][name]
	if ok {
		logger.Errorf("There is already a parameter stored with name %v\n", name)
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

// Update the value for a parameter given a market symbol, parameter name, and new parameter values.
// Throw an error if the parameter doesn't exist.
func (p *Params) Update(symbol string, name string, params interface{}) interface{} {
	_, ok := p.store[symbol][name]
	if ok {
		p.store[symbol][name] = params
		return params
	}
	logger.Errorf("There is no parameter stored with name %v\n", name)
	return nil
}

// Get the value for a parameter, given a market symbol and parameter name.
// Throw an error if the parameter does not exist.
func (p *Params) Get(symbol string, name string) interface{} {
	_, ok := p.store[symbol][name]
	if ok {
		return p.store[symbol][name]
	}
	logger.Errorf("There is no parameter stored with that name\n", name)
	return nil
}

// Get the names and values of all parameters for a given market symbol.
func (p *Params) GetAllParamsForSymbol(symbol string) map[string]interface{} {
	_, ok := p.store[symbol]
	if ok {
		return p.store[symbol]
	}
	logger.Errorf("There are no parameters stored with symbol %v\n", symbol)
	return nil
}

// Get the names and values of all parameters for all symbols.
func (p *Params) GetAllParams() map[string]interface{} {
	tmp := make(map[string]interface{})
	for key, _ := range p.store {
		for k, v := range p.store[key] {
			tmp[key+"-"+k] = v
		}
	}
	return tmp
}
