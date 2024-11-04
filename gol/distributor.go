package gol

import (
	"log"
	"net/rpc"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type Response struct {
	FinalWorld   [][]byte
	AliveCells   []util.Cell
	TurnsElapsed int
}

type AliveCellsResponse struct {
	NumAliveCells int
	TurnsElapsed  int
}


type GolRequest struct {
	Params Params
	World  [][]byte
}

type AliveCellsRequest struct {
	Params Params
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
quit := make(chan bool)
	c.ioCommand <- ioInput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	request := GolRequest{p, world}
	aliveCellsRequest := AliveCellsRequest{p,}

	turn := 0
	c.events <- StateChange{turn, Executing}

	client, err := rpc.Dial("tcp", ":8030")
	if err != nil {
		log.Fatalf("Error connecting to serer: %v", err)
	}
	defer func(client *rpc.Client) {
		err := client.Close()
		if err != nil {

		}
	}(client)

	var golResponse Response
	var aliveCellsResponse AliveCellsResponse

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

	err = client.Call("ParamService.GameSimulation", request, &golResponse)
	if err != nil {
		log.Fatalf("RPC error: %v", err)
	}



	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := golResponse.AliveCells
	turnsElapsed := golResponse.TurnsElapsed

	c.ioCommand<- ioOutput
	c.ioFilename<- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(golResponse.TurnsElapsed))

	for y:=0; y<p.ImageHeight; y++ {
		for x:=0; x<p.ImageWidth; x++ {
			c.ioOutput<- golResponse.FinalWorld[y][x]
		}
	}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	quit <- true

	c.events <- FinalTurnComplete{turnsElapsed, alive} //Uses FinalTurnComplete with calculateAliveCells

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
