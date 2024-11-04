package gol

import (
	"log"
	"net/rpc"
	"strconv"
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
	finalWorld [][]byte
	aliveCells []util.Cell
	turnsElapsed int
}

type golRequest struct {
	Params Params
	World [][]byte
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	c.ioCommand<- ioInput
	c.ioFilename<- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))



	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, p.ImageHeight)
	for i := range world { world[i] = make([]byte, p.ImageWidth) }

	for y:=0; y<p.ImageHeight; y++ {
		for x:=0; x<p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	request := golRequest{p, world}

	turn := 0
	c.events <- StateChange{turn, Executing}


	client, err := rpc.Dial("tcp", ":8030")
	if err != nil { log.Fatalf("Error connecting to serer: %v", err)}
	defer client.Close()

	var response Response
	client.Call("paramService.gameSimulation", request, &response)
	if err != nil {
		log.Fatal("RPC error:", err)
	}



	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := response.aliveCells
	turnsElapsed := response.turnsElapsed

	c.events <- FinalTurnComplete{turnsElapsed, alive} //Uses FinalTurnComplete with calculateAliveCells

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}





