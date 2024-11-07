package gol

import (
	"log"
	"net/rpc"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

// holds channels used for communication
type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// Response structure to contain final world state, cells alive at end and total turns
type Response struct {
	FinalWorld   [][]byte
	AliveCells   []util.Cell
	TurnsElapsed int
}

// AliveCellsResponse structure for counting alive cells at a specific turn
type AliveCellsResponse struct {
	NumAliveCells int
	TurnsElapsed  int
}

// GolRequest structure for initializing the GoL Simulation
type GolRequest struct {
	Params Params
	World  [][]byte
}

// AliveCellsRequest used to retrieve the number of alive cells during the simulation
type AliveCellsRequest struct {
	Params Params
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	quit := make(chan bool)

	// request initial world input
	c.ioCommand <- ioInput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	// populate world state from ioInput data
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	// initialize the requests
	request := GolRequest{p, world}
	aliveCellsRequest := AliveCellsRequest{p}

	// start simulation
	turn := 0
	c.events <- StateChange{turn, Executing}

	// establish rpc client connection
	client, err := rpc.Dial("tcp", "3.80.121.158")
	if err != nil {
		log.Fatalf("Error connecting to serer: %v", err)
	}
	defer func(client *rpc.Client) {
		err := client.Close()
		if err != nil {

		}
	}(client)

	// define response structures
	var golResponse Response
	var aliveCellsResponse AliveCellsResponse

	// ticker to report number of cells alive every 2 seconds
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				err = client.Call("ParamService.AliveCellsEvent", aliveCellsRequest, &aliveCellsResponse)
				if err != nil {
					log.Fatalf("RPC error: %v", err)
				}
				c.events <- AliveCellsCount{aliveCellsResponse.TurnsElapsed, aliveCellsResponse.NumAliveCells}

			}
		}
	}()

	// call RPC server to start and run GameSimulation
	err = client.Call("ParamService.GameSimulation", request, &golResponse)
	if err != nil {
		log.Fatalf("RPC error: %v", err)
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := golResponse.AliveCells
	turnsElapsed := golResponse.TurnsElapsed

	c.ioCommand <- ioOutput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(golResponse.TurnsElapsed))

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- golResponse.FinalWorld[y][x]
		}
	}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	quit <- true

	c.events <- FinalTurnComplete{turnsElapsed, alive} //Uses FinalTurnComplete with calculateAliveCells

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	close(c.events)
}
