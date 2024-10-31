package gol

import (
	"strconv"
	"sync"
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



// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	quit := make(chan bool)
	turn := 0
	var mutex sync.Mutex

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

	c.events <- StateChange{turn, Executing}



	go func() { {
		ticker := time.NewTicker(2 * time.Second)
		for {
			select {
			case <- quit:
				return
			case <-ticker.C:
				mutex.Lock()
				c.events <- AliveCellsCount{turn, len(calculateAliveCells(p, world))}
				mutex.Unlock()
			}
		}
	}}()

	// TODO: Execute all turns of the Game of Life.
	if p.Turns > 0 {
		for i := 0; i < p.Turns; i++ {
			mutex.Lock()
			world = calculateNextState(p, world) //Iterate through all turns
			turn++
			mutex.Unlock()
		}
	}

	quit <- true

	c.ioCommand<- ioOutput
	c.ioFilename<- (strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn))

	for y:=0; y<p.ImageHeight; y++ {
		for x:=0; x<p.ImageWidth; x++ {
			c.ioOutput<- world[y][x]
		}
	}

	c.ioCommand<- ioCheckIdle
	<-c.ioIdle


	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := calculateAliveCells(p, world)

	c.events <- FinalTurnComplete{turn, alive} //Uses FinalTurnComplete with calculateAliveCells

	

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

	channels := []chan []util.Cell{}

	if p.Threads == 1 {
		for y := 0; y < p.ImageHeight; y++ { //Iterate through all rows
			for x := 0; x < p.ImageWidth; x++ { //Iterate through all columns
				aliveNeighbours := aliveNeighbours(p, world, x, y) //Count alive neighbours using the function
				 if (shouldCellBeAlive(x, y, world, aliveNeighbours)) {alive = append(alive, util.Cell{x, y})}
			}
		}
	} else {
		for i := 0; i < p.Threads; i++ {
			startHeight := ((p.ImageHeight / p.Threads) * i)
			endHeight := ((p.ImageHeight / p.Threads) * (i + 1)) + p.ImageHeight%p.Threads
			out := make(chan []util.Cell)
			channels = append(channels, out)

			go worker(startHeight, endHeight, p.ImageWidth, world, p, out)
		}

		for j := 0; j < len(channels); j++ {
			alive = append(alive, <-channels[j]...)
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

func shouldCellBeAlive(x, y int, world [][]byte, aliveNeighbours int) bool {
	if world[y][x] == 255 && (aliveNeighbours == 2 || aliveNeighbours == 3) {
		return true
	} else if world[y][x] == 0 && aliveNeighbours == 3 {
		return true
	} else {return false}
}

func worker(startY, endY, endX int, world [][]byte, p Params, out chan<- []util.Cell) {
	aliveNextTurn := []util.Cell{}

	for y := startY; y < endY; y++ {
		for x := 0; x < endX; x++ {
			aliveNeighbours := aliveNeighbours(p, world, x, y) //Count alive neighbours using the function
			if (shouldCellBeAlive(x, y, world, aliveNeighbours)) {aliveNextTurn = append(aliveNextTurn, util.Cell{x, y})}

		}
	}

	out<- aliveNextTurn
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




