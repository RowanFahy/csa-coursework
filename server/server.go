package main

import (
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

// Params struct for how to run game of life
type Params struct {
	Turns       int
	ImageWidth  int
	ImageHeight int
}

// GolRequest holds request data for the RPC call
type GolRequest struct {
	Params Params
	World  [][]byte
}

// AliveCellsRequest used to retrieve the number of alive cells during the simulation
type AliveCellsRequest struct {
	Params Params
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

// ParamService struct that defines RPC method
type ParamService struct{}

// global variables
var world [][]byte
var turn = 0
var mutex = sync.Mutex{}

// GameSimulation RPC method that performs the Game of Life simulation
func (paramService *ParamService) GameSimulation(request *GolRequest, reply *Response) error {

	p := request.Params
	world = request.World

	if p.Turns > 0 {
		for i := 0; i < p.Turns; i++ {
			mutex.Lock()
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
	var alive []util.Cell
	newWorld := make([][]byte, len(world))
	copy(newWorld, world)

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			aliveNeighbours := aliveNeighbours(p, world, x, y)

			// Game of Life rules
			if world[y][x] == 255 && (aliveNeighbours == 2 || aliveNeighbours == 3) {
				alive = append(alive, util.Cell{X: x, Y: y}) // remain alive
			} else if world[y][x] == 0 && aliveNeighbours == 3 {
				alive = append(alive, util.Cell{X: x, Y: y}) // become alive
			}
		}
	}

	// Set newWorld to be 0
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			newWorld[y][x] = 0
		}
	}

	// Update newWorld with alive cells
	for _, aliveCell := range alive {
		newWorld[aliveCell.Y][aliveCell.X] = 255
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

// function used to send state of the game every 2 seconds
func (paramService *ParamService) AliveCellsEvent(request *AliveCellsRequest, reply *AliveCellsResponse) error {
	mutex.Lock()
	var alive []util.Cell
	for y := 0; y < request.Params.ImageHeight; y++ {
		for x := 0; x < request.Params.ImageWidth; x++ {
			if world[y][x] == 255 {
				alive = append(alive, util.Cell{X: x, Y: y}) // Collect alive cells
			}
		}
	}
	reply.NumAliveCells = len(alive)
	reply.TurnsElapsed = turn
	mutex.Unlock()

	return nil
}

// main starts the RPC server
func main() {
	paramService := new(ParamService)
	err := rpc.Register(paramService)
	if err != nil {
		return
	}

	ln, err := net.Listen("tcp", ":8030")
	handleError(err)
	defer func(ln net.Listener) {
		err := ln.Close()
		if err != nil {

		}
	}(ln) // Ensure listener is closed when exit
	fmt.Println("Listening on :8030")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting:", err.Error())
			continue
		}
		go rpc.ServeConn(conn)
	}
}
