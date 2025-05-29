package network

import (
	"client"
	"server"
)

func init() {
	client.ObtainServerPublicKey()

	go relay()
}

func relay() {
	defer close(server.Requests)
	defer close(client.Responses)
	for request := range client.Requests {
		server.Requests <- request
		b := <-server.Responses
		client.Responses <- (b)
	}
}
