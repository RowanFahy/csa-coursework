package gol

import "uk.ac.bris.cs/gameoflife/util"

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type cell struct {
	x, y int
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.
	world := make([][]byte, 0)

	turn := 0
	c.events <- StateChange{turn, Executing}

	// TODO: Execute all turns of the Game of Life.
	for i := 0; i < p.Turns; i++ {
		world = calculateNextState(p, world) //Iterate through all turns
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := calculateAliveCells(p, world)
	c.events <- FinalTurnComplete{p.Turns + 1, alive} //Uses FinalTurnComplete with calculateAliveCells

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
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

	for _, aliveCell := range alive {
		newWorld[aliveCell.Y][aliveCell.X] = 255 //For every cell that should be alive, set it to a value of 255
	}

	return newWorld
}

func aliveNeighbours(p Params, world [][]byte, x int, y int) int {
	sum := 0

	above := y - 1
	below := y + 1
	left := x - 1
	right := x + 1

	if above < 0 {
		above = p.ImageHeight - 1
	}
	if below > p.ImageHeight-1 {
		below = 0
	}
	if left < 0 {
		left = p.ImageWidth - 1
	}
	if right > p.ImageWidth-1 {
		right = 0
	}

	if world[above][left] == 255 {
		sum++
	}
	if world[above][x] == 255 {
		sum++
	}
	if world[above][right] == 255 {
		sum++
	}
	if world[y][left] == 255 {
		sum++
	}
	if world[y][right] == 255 {
		sum++
	}
	if world[below][left] == 255 {
		sum++
	}
	if world[below][x] == 255 {
		sum++
	}
	if world[below][right] == 255 {
		sum++
	}

	return sum //Return sum of alive neighbours
}

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	alive := make([]util.Cell, 0)

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				alive = append(alive, util.Cell{x, y})
			}
		}
	}
	return alive
}
