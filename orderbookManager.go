package main

import (
	"log"

	"github.com/sumorf/bitmex-api/swagger"
)

func updateLocalOrders(oldOrders []*swagger.Order, newOrders []*swagger.Order) []*swagger.Order {
	var updatedOrders []*swagger.Order
	for _, oldOrder := range oldOrders {
		found := false
		for _, newOrder := range newOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
				if newOrder.OrdStatus == "Canceled" || newOrder.OrdStatus == "Filled" || newOrder.OrdStatus == "Rejected" {
					log.Println(newOrder.OrdStatus, oldOrder.OrderID)
				} else {
					updatedOrders = append(updatedOrders, newOrder)
					// log.Println("Updated Order", newOrder.OrderID, newOrder.OrdStatus)
				}
			}
		}
		if !found {
			if oldOrder.OrdStatus == "Canceled" || oldOrder.OrdStatus == "Filled" || oldOrder.OrdStatus == "Rejected" {
				log.Println(oldOrder.OrdStatus, oldOrder.OrderID)
			} else {
				// log.Println("Old Order", oldOrder.OrderID)
				updatedOrders = append(updatedOrders, oldOrder)
			}
		}
	}

	for _, newOrder := range newOrders {
		found := false
		for _, oldOrder := range oldOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
			}
		}
		if !found {
			updatedOrders = append(updatedOrders, newOrder)
			log.Println("Adding Order", newOrder.OrderID, newOrder.OrdStatus)
		}
	}

	log.Println(len(updatedOrders), "orders")
	return updatedOrders
}
