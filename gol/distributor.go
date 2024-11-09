package gol

import (
	"fmt"
	"log"
	"net/rpc"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

/*
All tests that we passed for convenience

go test -v -run TestGol/-1$
go test -v -run TestAlive
go test -v -run TestPgm/-1$
*/

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

//Response struct to hold information needed from the server's Game of Life response
type Response struct {
	FinalWorld   [][]byte
	AliveCells   []util.Cell
	TurnsElapsed int
}

//Request struct to hold information that we need to send to the server
type GolRequest struct {
	Params Params
	World  [][]byte
}

//Response struct for getting aliveCells back when we use the ticker
type AliveCellsResponse struct {
	NumAliveCells int
	TurnsElapsed int
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	//Create channel for safely quitting goroutines
	quit := make(chan bool)

	//Give io the command to read an input file via ioCommand channel, then use ioFilename along with the width and height to find the correct input file
	filename := (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight))
	c.ioCommand <- ioInput
	c.ioFilename <- filename

	//Initialise World
	fmt.Println("Creating slice for world")
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	//Use the ioInput channel to receive the data from the input file and write it into world
	fmt.Println("Populating world")
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	//Initialise our request for the server
	request := GolRequest{p, world}

	//Notify io that we are going to begin executing the Game of Life
	c.events <- StateChange{0, Executing}

	//Dial the server with the IP and port of our AWS server
	//TODO MAKE SURE THE SERVER ADDRESS IS CORRECT AS IT CHANGES WHEN YOU INITIALISE A NEW SERVER
	fmt.Println("Dialling")
	client, err := rpc.Dial("tcp", ":8030")
	if err != nil {
		log.Fatalf("Error connecting to serer: %v", err)
	}
	defer func(client *rpc.Client) {
		err := client.Close()
		if err != nil {

		}
	}(client)

	//Initialise our response struct
	var response Response

	// Ticker to report number of cells alive every 2 seconds using an RPC call
	fmt.Println("Ticker function")
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			//Use quit channel to safely stop the goroutine and ticker if need be
			case <-quit:
				ticker.Stop()
				fmt.Println("Ticker Quitting")
				return
			case <-ticker.C:
				//Initialise a new response struct, then use an RPC request to get the required information from the server
				//Then send an AliveCellsCount event to io using this information
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

	//Use an RPC call to start running the game simulation on the server
	fmt.Println("Running GameSim")
	err = client.Call("ParamService.GameSimulation", request, &response)

	fmt.Println("GameSim Called")
	if err != nil {
		log.Fatalf("RPC error: %v", err)
	}
	fmt.Println("GameSim done")

	//Create a slice of alive cells using the response, and get the number of turns that had elapsed in the simulation
	alive := response.AliveCells
	turnsElapsed := response.TurnsElapsed

	//Output the .pgm file with the final state of the world
	outputPgm(c, response.FinalWorld, p, turnsElapsed)

	//Now that every turn has been processed, quit the ticker goroutine safely and make sure io is idle
	quit <- true
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	//Notify io that the final turn has been completed
	c.events <- FinalTurnComplete{turnsElapsed, alive}


	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	//Notify io that the program is quitting
	c.events <- StateChange{turnsElapsed, Quitting}
	fmt.Println("Done\n\n")

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

//outputPgm is used whenever we want to output a given board state as a .pgm file
func outputPgm(c distributorChannels, world [][]byte, p Params, turn int) {

	//Give io the command to output a .pgm file and give it the correct filename based on the baord dimensions and how many turns have been completed at this point
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	c.ioCommand <- ioOutput
	c.ioFilename <- filename

	//Iterate through all the cells and send them down the ioOutput channel for io to use to output the .pgm file
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}

	//Make sure io is idle before doing anything else
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	//Once io is idle and we know it isn't computing anything, notify io that the image output has been completed using the ImageOutputComplete event
	c.events <- ImageOutputComplete{turn, filename}
}
