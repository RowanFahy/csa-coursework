package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

// Params struct holds parameters for the Game of Life simulation
type Params struct {
	Turns       int
	ImageWidth  int
	ImageHeight int
}

// GolRequest struct holds the request data for the RPC call
type GolRequest struct { // Changed to exported type
	Params Params
	World  [][]byte
}

// Response struct holds the response data to be sent back to the client
type Response struct {
	FinalWorld   [][]byte    // Exported field
	AliveCells   []util.Cell // Exported field
	TurnsElapsed int         // Exported field
}

//AliveCellsResponse holds response data for whenever we want to report the number of alive cells
//Which in our case is done using a ticker on the client side
type AliveCellsResponse struct {
	NumAliveCells int
	TurnsElapsed int
}

// ParamService is the struct that defines the RPC methods (exported)
type ParamService struct{}

//Declare the global variables world and turn so that we can report number of alive cells
//Since otherwise there is no way for AliveCellsEvent to access world or turn
//Also declare our mutex to help prevent race conditions
var world [][]byte
var turn int
var mutex sync.Mutex

// GameSimulation is the RPC method that performs the Game of Life simulation
func (ps *ParamService) GameSimulation(request *GolRequest, reply *Response) error { // Changed to exported type

	//Initialise world to be the same as in the request, as well as turn and setting p as the request params to make life easier
	p := request.Params
    initialiseWorld(p)
	world = request.World
	turn = 0

	//Lock the mutex and calculate the next state of the world, then set it to be world
	//Increment the turn counter before unlocking the mutex
	//This repeats for the number of turns specified in Params
	if p.Turns > 0 {
		for i := 0; i < p.Turns; i++ {
			mutex.Lock()
			fmt.Printf("Executing turn %d\n", i)
			world = calculateNextState(p, world)
			turn++
			mutex.Unlock()
		}
	}

	// Calculate the alive cells in the final state
	alive := calculateAliveCells(p, world)

	// Populate the reply with the results
	reply.AliveCells = alive
	reply.FinalWorld = world
	reply.TurnsElapsed = turn
	return nil
}

// calculateNextState calculates the next state of the world based on the current state
func calculateNextState(p Params, world [][]byte) [][]byte {
	//Make a slice of alive cells, and initialise a new world to return
	var alive []util.Cell
	newWorld := make([][]byte, len(world))
	copy(newWorld, world)

	//Iterate through all cells and count how many alive neighbours they have
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			aliveNeighbours := aliveNeighbours(p, world, x, y)

			// Apply Game of Life rules. If a cell should be alive next turn, add it to the slice of alive cells
			if world[y][x] == 255 && (aliveNeighbours == 2 || aliveNeighbours == 3) {
				alive = append(alive, util.Cell{X: x, Y: y})
			} else if world[y][x] == 0 && aliveNeighbours == 3 {
				alive = append(alive, util.Cell{X: x, Y: y})
			}
		}
	}

	// Reset newWorld to be 0 across the board
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			newWorld[y][x] = 0
		}
	}

	// Update newWorld with alive cells
	for _, aliveCell := range alive {
		newWorld[aliveCell.Y][aliveCell.X] = 255
	}

	//Return newWorld
	return newWorld
}

// aliveNeighbours counts the alive neighbours of a given cell
func aliveNeighbours(p Params, world [][]byte, x int, y int) int {
	//Initialise the sum of alive neighbours as 0
	sum := 0

	//Iterate through the 8 cells surrounding the inputed cell, starting with the top left, then going towards top rmiddle, top right, middle left etc
	//When i and j are 0, it means it is checking if itself is alive or dead which we don't want, so we use continue
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			//The ny and nx are defined in ways so that if the surrounding cell is "off" the board (outside the height/width in params), it wraps around to the other side of the board
			//After that, if the surrounding cell is alive, we increment the sum of alive neighbours.
			ny := (y + i + p.ImageHeight) % p.ImageHeight
			nx := (x + j + p.ImageWidth) % p.ImageWidth
			if world[ny][nx] == 255 {
				sum++
			}
		}
	}
	//After going through all surrounding cells, return the number of alive neighbours
	return sum
}

// calculateAliveCells returns a list of alive cells in the world
func calculateAliveCells(p Params, world [][]byte) []util.Cell {

	//Create a slice for storing the alive cells
	var alive []util.Cell

	//Iterate through every cell. If it's value is 255 (alive), add it to the slice of alive cells
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				alive = append(alive, util.Cell{X: x, Y: y})
			}
		}
	}

	//Finally, return the slice of alive cells
	return alive
}

// handleError prints error messages
func handleError(err error) {
	if err != nil {
		fmt.Println("error:", err)
	}
}

//Function to handle the response when we request the number of alive cells
func (ps *ParamService) AliveCellsEvent(request *GolRequest,reply *AliveCellsResponse) error {

	//Lock the mutex to avoid race conditions and initialise the number of alive cells
	mutex.Lock()
	sum := 0

	//Iterate through every cell and add all alive cells to the slice of alive cells
	for y := 0; y < request.Params.ImageHeight; y++ {
		for x := 0; x < request.Params.ImageWidth; x++ {
			if world[y][x] == 255 {
				sum++
			}
		}
	}

	//Populate the response with the number of alive cells and number of turns elapsed
	reply.NumAliveCells = sum
	reply.TurnsElapsed = turn

	//Unlock the mutex
	mutex.Unlock()
	return nil
}

//Initialise the world correctly to avoid errors
func initialiseWorld(p Params) {
		fmt.Println("Initialising World")
		world = make([][]byte, p.ImageHeight)
		for i := range world {
			world[i] = make([]byte, p.ImageWidth)
		}


	for y:= 0; y < p.ImageHeight; y++ {
		for x:= 0; x < p.ImageWidth; x++ {
			world[y][x] = 0
		}
	}
}

// main starts the RPC server
func main() {
	//Create a new ParamService instance and register it
	paramService := new(ParamService)
	err := rpc.Register(paramService)
	if err != nil {
		return
	}

	//Listen on port :8030 and ensure the listener is closed on exit
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	ln, err := net.Listen("tcp", ":"+*pAddr) // Listen on port 8030
	handleError(err)
	defer func(ln net.Listener) {
		err := ln.Close()
		if err != nil {

		}
	}(ln)
	fmt.Println("Listening on :8030")


	for {
		// Accept incoming connections and  serve each connection concurrently
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting:", err.Error())
			continue
		}
		go rpc.ServeConn(conn)
	}
}