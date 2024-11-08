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

type AliveCellsResponse struct {
	numAliveCells int
	turnsElapsed int
}

// ParamService is the struct that defines the RPC methods (exported)
type ParamService struct{}

var world [][]byte
var turn int
var mutex sync.Mutex

// GameSimulation is the RPC method that performs the Game of Life simulation
func (ps *ParamService) GameSimulation(request *GolRequest, reply *Response) error { // Changed to exported type

	p := request.Params

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = request.World[y][x]
		}
	}
	turn = 0

	// Execute all turns of the Game of Life
	if p.Turns > 0 {
		for i := 0; i < p.Turns; i++ {
			mutex.Lock()
			world = calculateNextState(p, world) // Iterate through all turns
			turn++                               // Increment the turn counter
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
	var alive []util.Cell                  // Make a slice of alive cells
	newWorld := make([][]byte, len(world)) // Create a new world to return
	copy(newWorld, world)                  // Copy current world to newWorld

	for y := 0; y < p.ImageHeight; y++ { // Iterate through all rows
		for x := 0; x < p.ImageWidth; x++ { // Iterate through all columns
			aliveNeighbours := aliveNeighbours(p, world, x, y) // Count alive neighbours

			// Apply Game of Life rules
			if world[y][x] == 255 && (aliveNeighbours == 2 || aliveNeighbours == 3) {
				alive = append(alive, util.Cell{X: x, Y: y}) // Cell remains alive
			} else if world[y][x] == 0 && aliveNeighbours == 3 {
				alive = append(alive, util.Cell{X: x, Y: y}) // Cell becomes alive
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
		newWorld[aliveCell.Y][aliveCell.X] = 255 // Set alive cells to 255
	}

	return newWorld
}

// aliveNeighbours counts the alive neighbours of a given cell
func aliveNeighbours(p Params, world [][]byte, x int, y int) int {
	sum := 0

	// Check all 8 possible neighbours
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 { // Skip the current cell
				continue
			}
			ny := (y + i + p.ImageHeight) % p.ImageHeight
			nx := (x + j + p.ImageWidth) % p.ImageWidth
			if world[ny][nx] == 255 {
				sum++ // Count alive neighbours
			}
		}
	}

	return sum // Return total alive neighbours
}

// calculateAliveCells returns a list of alive cells in the world
func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	var alive []util.Cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				alive = append(alive, util.Cell{X: x, Y: y}) // Collect alive cells
			}
		}
	}
	return alive
}

// handleError prints error messages
func handleError(err error) {
	if err != nil {
		fmt.Println("error:", err)
	}
}

func (ps *ParamService) AliveCellsEvent(request *GolRequest, reply *AliveCellsResponse) error {
	mutex.Lock()
	var alive []util.Cell
	p := request.Params
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				alive = append(alive, util.Cell{X: x, Y: y}) // Collect alive cells
			}
		}
	}

	reply.numAliveCells = len(alive)
	reply.turnsElapsed = turn
	mutex.Unlock()
	return nil
}

// main starts the RPC server
func main() {
	paramService := new(ParamService) // Create a new ParamService instance
	err := rpc.Register(paramService)
	if err != nil {
		return
	} // Register the service

	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	ln, err := net.Listen("tcp", ":"+*pAddr) // Listen on port 8030
	handleError(err)
	defer func(ln net.Listener) {
		err := ln.Close()
		if err != nil {

		}
	}(ln) // Ensure the listener is closed on exit
	fmt.Println("Listening on :8030")

	for {
		conn, err := ln.Accept() // Accept incoming connections
		if err != nil {
			fmt.Println("Error accepting:", err.Error())
			continue
		}
		go rpc.ServeConn(conn) // Serve each connection concurrently
	}
}