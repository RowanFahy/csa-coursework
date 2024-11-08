package gol

import (
	"fmt"
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

type GolRequest struct {
	Params Params
	World  [][]byte
}

type AliveCellsResponse struct {
	NumAliveCells int
	TurnsElapsed int
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	quit := make(chan bool)
	c.ioCommand <- ioInput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))

	fmt.Println("Creating slice for world")
	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}
	fmt.Println("Populating world")
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	request := GolRequest{p, world}

	turn := 0
	c.events <- StateChange{turn, Executing}
	fmt.Println("Dialling")
	client, err := rpc.Dial("tcp", "3.84.28.129:8030")
	if err != nil {
		log.Fatalf("Error connecting to serer: %v", err)
	}
	defer func(client *rpc.Client) {
		err := client.Close()
		if err != nil {

		}
	}(client)

	var response Response

	fmt.Println("Ticker function")
	// ticker to report number of cells alive every 2 seconds
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case <-quit:
				ticker.Stop()
				fmt.Println("Ticker Quitting")
				return
			case <-ticker.C:
				var aliveCellsResponse AliveCellsResponse
				fmt.Println("Requesting Alive Cells")
				err = client.Call("ParamService.AliveCellsEvent", request, &aliveCellsResponse)
				if err != nil {
					log.Fatalf("RPC error: %v", err)
				}
				c.events <- AliveCellsCount{aliveCellsResponse.TurnsElapsed, aliveCellsResponse.NumAliveCells}

			}
		}
	}()
	fmt.Println("Running GameSim")
	err = client.Call("ParamService.GameSimulation", request, &response)

	fmt.Println("GameSim Called")
	if err != nil {
		log.Fatalf("RPC error: %v", err)
	}
	fmt.Println("GameSim done")
	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := response.AliveCells
	turnsElapsed := response.TurnsElapsed

	c.ioCommand <- ioOutput
	c.ioFilename <- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(response.TurnsElapsed))
	fmt.Println("Outputting World")
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- response.FinalWorld[y][x]
		}
	}


	quit <- true
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- FinalTurnComplete{turnsElapsed, alive} //Uses FinalTurnComplete with calculateAliveCells


	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	fmt.Println("Done\n\n")
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}