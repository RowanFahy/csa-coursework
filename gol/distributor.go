package gol

import (
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

/*
All tests for convenience

go test -v -run TestGol
go test -v -run TestAlive
go test -v -run TestPgm
go test -v -run TestKeyboard -sdl
go test -v -run TestSdl -sdl
go test -v -race
go run .
*/

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	//Creating channels used to safely quit goroutines, initialise turn and creating our mutex lock
	quit := make(chan bool)
	quitComputation := make(chan bool)
	turn := 0
	var mutex sync.Mutex

	//Give io the command to read an input file via ioCommand channel, then use ioFilename along with the width and height to find the correct input file
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioCommand <- ioInput
	c.ioFilename <- filename

	//Initialise World
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	//Use the ioInput channel to receive the data from the input file and write it into world
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
			if world[y][x] == 255 {c.events<-CellFlipped{0, util.Cell{x, y}}}
		}
	}

	//Notify io that the program will begin executing using the events channel
	c.events <- StateChange{turn, Executing}

	//Go function to handle any key presses
	go func() {
		isPaused := false
		for {
			select {
			case x := <-c.keyPresses:
				//Handle s key press, using a mutex lock if unpaused to stop race conditions
				//Mutex not needed if paused as world and turn will not be updated + mutex cannot lock since we pause the simulation by locking it with p
				if x == 's' {
					if isPaused == true {
						outputPgm(c, world, p, turn)
					} else if isPaused == false {
						mutex.Lock()
						outputPgm(c, world, p, turn)
						mutex.Unlock()
					}
				}
				//If paused, pressing q unpauses then immediately sends true down quitComputation channel which stops world from being updated further
				//If unpaused it just sends true down quitComputation
				if x == 'q' {
					if isPaused == true {
						mutex.Unlock()
						quitComputation <- true
					} else if isPaused == false {
						quitComputation <- true
					}
				}
				//If unpaused, takes up the mutex lock which stops world and turn from being updated, effectively pausing the simulation. Also sends a stateChange event to notify io
				//If paused, frees the mutex lock, allowing world and turn to update again, resuming the simulation. Also sends a stateChange event to notify io
				if x == 'p' {
					if isPaused == false {
						mutex.Lock()
						c.events <- StateChange{turn, Paused}
						isPaused = true
					} else if isPaused == true {
						c.events <- StateChange{turn, Executing}
						isPaused = false
						mutex.Unlock()
					}
				}

			}

		}
	}()

	//Go function to handle the ticker to report the number of alive cells every 2 seconds
	go func() {
			ticker := time.NewTicker(2 * time.Second)
			for {
				select {
				//Uses quit channel to safely stop the goroutine
				case <-quit:
					ticker.Stop()
					return
				case <-ticker.C:
					//When the ticker ticks every 2 seconds, lock the mutex to stop world and turn from being updated
					//then use them to send an aliveCellsCount event to io, then unlock the mutex
					mutex.Lock()
					c.events <- AliveCellsCount{turn, len(calculateAliveCells(p, world))}
					mutex.Unlock()
				}
			}

	}()

	//Anonymous function to handle updating the state of the world while also allowing for the ability to halt computation if need be via the quitComputation channel
	func() {
		for i := 0; i < p.Turns; i++ {
			select {
			//If true is received from the quitComputation channel via pressing q, return the function which stops any more turns from being processed
			case <-quitComputation:
				return
			default:
				//Otherwise, lock the mutex, calculate the next state of the world and set it to be the world
				//Then increment the turn counter and notify io that a turn has been completed before unlocking the mutex
				//This repeats for the number of turns specified in Params, unless quitComputation sends true
				mutex.Lock()
				world = calculateNextState(p, world, c, turn)
				turn++
				c.events <- TurnComplete{turn}
				mutex.Unlock()
			}
		}
	}()

	//Now that every turn has been processed (or quitComputation sent true), quit the ticker goroutine safely
	quit <- true

	//Lock the mutex in case of race conditions, then output the .pgm file of the final state of the world before unlocking the mutex
	mutex.Lock()
	outputPgm(c, world, p, turn)
	mutex.Unlock()

	//Make sure io is idle before doing anything else with it
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	//Lock the mutex again in case of race conditions somehow, then calculate the list of alive cells before unlocking the mutex
	mutex.Lock()
	alive := calculateAliveCells(p, world)
	mutex.Unlock()

	//Notify io that the final turn has been completed
	c.events <- FinalTurnComplete{turn, alive}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	//Notify io that the program is quitting
	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

//calculateNextState does what it says, it splits up the work into various goroutines so that it can calculate the next state of the board
func calculateNextState(p Params, world [][]byte, c distributorChannels, turn int) [][]byte {
	//Make an empty slice of alive cells, a slice for every cell that has been flipped (changed state from this turn to the next)
	//as well as initialise a newWorld to return at the end of the function
	var alive []util.Cell
	var flippedCells []util.Cell

	newWorld := make([][]byte, p.ImageHeight)
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageWidth)
	}

	//Make a slice channels of type cell slice
	var channels []chan []util.Cell

	//If there is only one thread:
	if p.Threads == 1 {
		//Iterate through all rows and columns, then count the number of alive neighbours using the helper function
		//Then use the number of alive neighbours to determine if the current cell should be alive or dead for the next turn
		//If it should be alive, add it to the list of alive cells
		for y := 0; y < p.ImageHeight; y++ {
			for x := 0; x < p.ImageWidth; x++ {
				aliveNeighbours := aliveNeighbours(p, world, x, y)
				if shouldCellBeAlive(x, y, world, aliveNeighbours) {
					alive = append(alive, util.Cell{x, y})
				}
			}
		}
	} else {

		//If there is more than one thread:
		for i := 0; i < p.Threads; i++ {

			//Split up the board into strips, the size of which depending on how many threads we are using
			//Make a channel of a slice of cells and add it to the slice of channels - This ensures that we wait for every goroutine to give it's output through it's respective channel
			startHeight := (p.ImageHeight / p.Threads) * i
			endHeight := ((p.ImageHeight / p.Threads) * (i + 1)) + p.ImageHeight % p.Threads
			out := make(chan []util.Cell)
			channels = append(channels, out)

			//Start the worker using it's start and end height, as well as the channel it should output to
			go worker(startHeight, endHeight, p.ImageWidth, world, p, out)
		}

		//Iterate through every channel made for eah goroutine, and once they send their slice of cells append it to the total alive cell slice
		for j := 0; j < len(channels); j++ {
			alive = append(alive, <-channels[j]...)
		}
	}

	//Make sure our newWorld we are returning is 0 across the board in case it has any values in it for whatever reason
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			newWorld[y][x] = 0
		}
	}

	//Go through every cell in the full slice of alive cells and set the corresponding cell in newWorld to be alive for the next turn
	for _, aliveCell := range alive {
		newWorld[aliveCell.Y][aliveCell.X] = 255
	}

	//Since we now have the "old" world from the current turn, and the newWorld for the next turn, compare the values of each cell
	//If they have a different value, then we know they are going to change state when we go to the next turn, so add it to the slice of flipped cells
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] != newWorld[y][x] {
				flippedCells = append(flippedCells, util.Cell{x, y})
			}
		}
	}

	//For every cell that has flipped, send the CellsFlipped event to io
	c.events<- CellsFlipped{turn, flippedCells}

	//Finally return the newWorld
	return newWorld
}

//This worker function calculates which cells from it's given strip should be alive next turn
func worker(startY, endY, endX int, world [][]byte, p Params, out chan<- []util.Cell) {

	//Create a slice of cells that should be alive next turn
	var aliveNextTurn []util.Cell

	//Iterate through the row(s) the goroutine has been given and count how many alive neighbours each cell in the row has
	//Then use the shouldCellBeAlive function to decide if it should be alive or dead next turn
	//If the cell should be alive, add it to the aliveNextTurn slice
	for y := startY; y < endY; y++ {
		for x := 0; x < endX; x++ {
			aliveNeighbours := aliveNeighbours(p, world, x, y)
			if shouldCellBeAlive(x, y, world, aliveNeighbours) {
				aliveNextTurn = append(aliveNextTurn, util.Cell{x, y})
			}

		}
	}

	//Finally, send the aliveNextTurn slice down it's output channel
	out <- aliveNextTurn
}

//aliveNeighbours iterates through all of the neighbours of a given cell to find out how many are alive
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

//shouldCellBeAlive tells us if a cell should be alive or dead next turn
func shouldCellBeAlive(x, y int, world [][]byte, aliveNeighbours int) bool {
	//Uses the main ruleset of the Game of Life to determine if the cells should be alive or dead for the next turn
	/*
	any live cell with fewer than two live neighbours dies
	any live cell with two or three live neighbours is unaffected
	any live cell with more than three live neighbours dies
	any dead cell with exactly three live neighbours becomes alive
	*/
	//In this case, true means it should be alive next turn and false means it should not be
	if world[y][x] == 255 && (aliveNeighbours == 2 || aliveNeighbours == 3) {
		return true
	} else if world[y][x] == 0 && aliveNeighbours == 3 {
		return true
	} else {
		return false
	}
}

//calculateAliveCells goes through the board to find out which cells are alive, then returns them all in a slice
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
