package server

import (
	"fmt"
	"net"
	"net/rpc"
	"uk.ac.bris.cs/gameoflife/util"
)

type Params struct {
	Turns int
	ImageWidth  int
	ImageHeight int
}

type Response struct {
	Message string
}

type paramService struct {}

func (ps *paramService) gameSimulation(p Params, world [][]byte) {
	turn := 0
	// TODO: Execute all turns of the Game of Life.
	if p.Turns > 0 {
		for i := 0; i < p.Turns; i++ {
			world = calculateNextState(p, world) //Iterate through all turns
			turn++
		}
	}
	alive := calculateAliveCells(p, world)

}

func calculateNextState(p Params, world [][]byte) [][]byte {

	var alive []util.Cell                  //Make a slice of alive cells
	newWorld := make([][]byte, len(world)) //Make a new world to return
	copy(newWorld, world)                  //Copy world to newWorld

	for y := 0; y < p.ImageHeight; y++ { //Iterate through all rows
		for x := 0; x < p.ImageWidth; x++ { //Iterate through all columns
			aliveNeighbours := aliveNeighbours(p, world, x, y) //Count alive neighbours using the function

			if world[y][x] == 255 && (aliveNeighbours == 2 || aliveNeighbours == 3) {
				alive = append(alive, util.Cell{x, y}) //Rule for alive cells to stay alive
			} else if world[y][x] == 0 && aliveNeighbours == 3 {
				alive = append(alive, util.Cell{x, y}) //Rule for dead cells to become alive
			}
		}
	}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			newWorld[y][x] = 0 //Reset newWorld to be 0 across the board
		}
	}

	for _, aliveCell := range alive { newWorld[aliveCell.Y][aliveCell.X] = 255 } //For every cell that should be alive, set it to a value of 255

	return newWorld
}

func aliveNeighbours(p Params, world [][]byte, x int, y int) int {
	sum := 0

	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			ny := (y + i + p.ImageHeight) % p.ImageHeight
			nx := (x + j + p.ImageWidth) % p.ImageWidth
			if world[ny][nx] == 255 { sum++ }
		}
	}

	return sum //Return sum of alive neighbours
}

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	var alive []util.Cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				alive = append(alive, util.Cell{X: x, Y: y})
			}
		}
	}
	return alive
}

func handleError(err error) {
	if err != nil {
		fmt.Println("error")
		return
	}
}

func main() {
	paramService := new(paramService)
	rpc.Register(paramService)

	ln, err := net.Listen("tcp", ":8080")
	handleError(err)
	defer ln.Close()
	fmt.Println("Listening on :8080")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			continue
		}
		go rpc.ServeConn(conn)
	}
}