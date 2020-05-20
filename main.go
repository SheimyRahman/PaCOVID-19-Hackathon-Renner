package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/danicat/simpleansi"
)

var (
	configFile = flag.String("config-file", "config.json", "path to custom configuration file")
	mazeFile   = flag.String("maze-file", "maze01.txt", "path to a custom maze file")
)

type sprite struct {
	row      int
	col      int
	startRow int
	startCol int
}

type virus struct {
	position sprite
	status   VirusStatus
}

type VirusStatus string

type zombie struct {
	position sprite
	status   ZombieStatus
}

type ZombieStatus string

const (
	VirusStatusNormal  VirusStatus  = "Normal"
	VirusStatusBlue    VirusStatus  = "Blue"
	ZombieStatusNormal ZombieStatus = "Normal"
	ZombieStatusBlue   ZombieStatus = "Blue"
)

var virussStatusMx sync.RWMutex
var zombiesStatusMx sync.RWMutex
var washMx sync.Mutex
var foodMx sync.Mutex

type config struct {
	Player           string        `json:"player"`
	StartFlag        string        `json:"startflag"`
	Virus            string        `json:"virus"`
	VirusBlue        string        `json:"virus_blue"`
	Wall             string        `json:"wall"`
	Dot              string        `json:"dot"`
	Wash             string        `json:"wash"`
	Death            string        `json:"death"`
	Space            string        `json:"space"`
	Food             string        `json:"food"`
	Zombie           string        `json:"zombie"`
	ZombieBlue       string        `json:"zombie_blue"`
	People           string        `json:"people"`
	UseEmoji         bool          `json:"use_emoji"`
	WashDurationSecs time.Duration `json:"wash_duration_secs"`
	FoodDurationSecs time.Duration `jason:"food_duration_secs"`
}

var cfg config
var player sprite
var viruss []*virus
var zombies []*zombie
var maze []string
var score int
var numDots int
var lives = 3

func loadConfig(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&cfg)
	if err != nil {
		return err
	}

	return nil
}

func loadMaze(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		maze = append(maze, line)
	}

	for row, line := range maze {
		for col, char := range line {
			switch char {
			case 'P':
				player = sprite{row, col, row, col}
			case 'V':
				viruss = append(viruss, &virus{sprite{row, col, row, col}, VirusStatusNormal})
			case 'Z':
				zombies = append(zombies, &zombie{sprite{row, col, row, col}, ZombieStatusNormal})
			case '.':
				numDots++
			}
		}
	}

	return nil
}

func moveCursor(row, col int) {
	if cfg.UseEmoji {
		simpleansi.MoveCursor(row, col*2)
	} else {
		simpleansi.MoveCursor(row, col)
	}
}

func printScreen() {
	simpleansi.ClearScreen()
	for _, line := range maze {
		for _, chr := range line {
			switch chr {
			case 'W':
				fmt.Print(simpleansi.WithBlueBackground(cfg.Wall))
			case '.':
				fmt.Print(cfg.Dot)
			case 'X':
				fmt.Print(cfg.Wash)
			case 'Y':
				fmt.Print(cfg.People)
			case 'O':
				fmt.Print(cfg.Food)
			case 'F':
				fmt.Print(cfg.StartFlag)
			default:
				fmt.Print(cfg.Space)
			}
		}
		fmt.Println()
	}

	moveCursor(player.row, player.col)
	fmt.Print(cfg.Player)

	virussStatusMx.RLock()
	for _, v := range viruss {
		moveCursor(v.position.row, v.position.col)
		if v.status == VirusStatusNormal {
			fmt.Printf(cfg.Virus)
		} else if v.status == VirusStatusBlue {
			fmt.Printf(cfg.VirusBlue)
		}
	}
	virussStatusMx.RUnlock()

	zombiesStatusMx.RLock()
	for _, z := range zombies {
		moveCursor(z.position.row, z.position.col)
		if z.status == ZombieStatusNormal {
			fmt.Printf(cfg.Zombie)
		} else if z.status == ZombieStatusBlue {
			fmt.Printf(cfg.ZombieBlue)
		}
	}
	zombiesStatusMx.RUnlock()

	moveCursor(len(maze)+1, 0)

	livesRemaining := strconv.Itoa(lives) //converts lives int to a string
	if cfg.UseEmoji {
		livesRemaining = getLivesAsEmoji()
	}

	fmt.Println("Score:", score, "\tLives:", livesRemaining)
}

//concatenate the correct number of player emojis based on lives
func getLivesAsEmoji() string {
	buf := bytes.Buffer{}
	for i := lives; i > 0; i-- {
		buf.WriteString(cfg.Player)
	}
	return buf.String()
}

func readInput() (string, error) {
	buffer := make([]byte, 100)

	cnt, err := os.Stdin.Read(buffer)
	if err != nil {
		return "", err
	}

	if cnt == 1 && buffer[0] == 0x1b {
		return "ESC", nil
	} else if cnt >= 3 {
		if buffer[0] == 0x1b && buffer[1] == '[' {
			switch buffer[2] {
			case 'A':
				return "UP", nil
			case 'B':
				return "DOWN", nil
			case 'C':
				return "RIGHT", nil
			case 'D':
				return "LEFT", nil
			}
		}
	}

	return "", nil
}

func makeMove(oldRow, oldCol int, dir string) (newRow, newCol int) {
	newRow, newCol = oldRow, oldCol

	switch dir {
	case "UP":
		newRow = newRow - 1
		if newRow < 0 {
			newRow = len(maze) - 1
		}
	case "DOWN":
		newRow = newRow + 1
		if newRow == len(maze)-1 {
			newRow = 0
		}
	case "RIGHT":
		newCol = newCol + 1
		if newCol == len(maze[0]) {
			newCol = 0
		}
	case "LEFT":
		newCol = newCol - 1
		if newCol < 0 {
			newCol = len(maze[0]) - 1
		}
	}

	if maze[newRow][newCol] == 'W' {
		newRow = oldRow
		newCol = oldCol
	}

	return
}

func movePlayer(dir string) {
	player.row, player.col = makeMove(player.row, player.col, dir)

	removeDot := func(row, col int) {
		maze[row] = maze[row][0:col] + " " + maze[row][col+1:]
	}

	switch maze[player.row][player.col] {
	case '.':
		numDots--
		score++
		removeDot(player.row, player.col)
	case 'X':
		score += 10
		removeDot(player.row, player.col)
		go processWash()
	case 'Y':
		score -= 10
		removeDot(player.row, player.col)
		go processWash2()
	case 'O':
		score += 25
		removeDot(player.row, player.col)
		go processFood()
	}
}

func updateViruss(viruss []*virus, virusStatus VirusStatus) {
	virussStatusMx.Lock()
	defer virussStatusMx.Unlock()
	for _, v := range viruss {
		v.status = virusStatus
	}
}

func updateZombies(zombies []*zombie, zombieStatus ZombieStatus) {
	zombiesStatusMx.Lock()
	defer zombiesStatusMx.Unlock()
	for _, z := range zombies {
		z.status = zombieStatus
	}
}

var washTimer *time.Timer

func processWash() {
	washMx.Lock()
	updateViruss(viruss, VirusStatusBlue)
	if washTimer != nil {
		washTimer.Stop()
	}
	washTimer = time.NewTimer(time.Second * cfg.WashDurationSecs)
	washMx.Unlock()
	<-washTimer.C
	washMx.Lock()
	washTimer.Stop()
	updateViruss(viruss, VirusStatusNormal)
	washMx.Unlock()
}

func processWash2() {
	washMx.Lock()
	updateZombies(zombies, ZombieStatusBlue)
	if washTimer != nil {
		washTimer.Stop()
	}
	washTimer = time.NewTimer(time.Second * cfg.WashDurationSecs)
	washMx.Unlock()
	<-washTimer.C
	washMx.Lock()
	washTimer.Stop()
	updateZombies(zombies, ZombieStatusNormal)
	washMx.Unlock()
}

// Food appears
var foodTimer *time.Timer

func processFood() {
	foodMx.Lock()
	updateViruss(viruss, VirusStatusNormal)
	if foodTimer != nil {
		foodTimer.Stop()
	}
	foodTimer = time.NewTimer(time.Second * cfg.FoodDurationSecs)
	foodMx.Unlock()
	<-foodTimer.C
	foodMx.Lock()
	foodTimer.Stop()
	updateViruss(viruss, VirusStatusNormal)
	foodMx.Unlock()
}

func drawDirection() string {
	dir := rand.Intn(4)
	move := map[int]string{
		0: "UP",
		1: "DOWN",
		2: "RIGHT",
		3: "LEFT",
	}
	return move[dir]
}

func moveViruss() {
	for _, v := range viruss {
		dir := drawDirection()
		v.position.row, v.position.col = makeMove(v.position.row, v.position.col, dir)
	}
}

func moveZombies() {
	for _, z := range zombies {
		dir := drawDirection()
		z.position.row, z.position.col = makeMove(z.position.row, z.position.col, dir)
	}
}

func initialise() {
	cbTerm := exec.Command("stty", "cbreak", "-echo")
	cbTerm.Stdin = os.Stdin

	err := cbTerm.Run()
	if err != nil {
		log.Fatalln("unable to activate cbreak mode:", err)
	}
}

func cleanup() {
	cookedTerm := exec.Command("stty", "-cbreak", "echo")
	cookedTerm.Stdin = os.Stdin

	err := cookedTerm.Run()
	if err != nil {
		log.Fatalln("unable to activate cooked mode:", err)
	}
}

func main() {
	flag.Parse()

	// initialize game
	initialise()
	defer cleanup()

	// load resources
	err := loadMaze(*mazeFile)
	if err != nil {
		log.Println("failed to load maze:", err)
		return
	}

	err = loadConfig(*configFile)
	if err != nil {
		log.Println("failed to load configuration:", err)
		return
	}

	// process input (async)
	input := make(chan string)
	go func(ch chan<- string) {
		for {
			input, err := readInput()
			if err != nil {
				log.Print("error reading input:", err)
				ch <- "ESC"
			}
			ch <- input
		}
	}(input)

	// game loop
	for {
		// process movement
		select {
		case inp := <-input:
			if inp == "ESC" {
				lives = 0
			}
			movePlayer(inp)
		default:
		}

		moveViruss()

		// process collisions
		for _, v := range viruss {
			if player.row == v.position.row && player.col == v.position.col {
				virussStatusMx.RLock()
				if v.status == VirusStatusNormal {
					lives = lives - 1
					if lives != 0 {
						moveCursor(player.row, player.col)
						fmt.Print(cfg.Death)
						moveCursor(len(maze)+2, 0)
						virussStatusMx.RUnlock()
						updateViruss(viruss, VirusStatusNormal)
						time.Sleep(1000 * time.Millisecond) //dramatic pause before reseting player position
						player.row, player.col = player.startRow, player.startCol
					}
				} else if v.status == VirusStatusBlue {
					virussStatusMx.RUnlock()
					updateViruss([]*virus{v}, VirusStatusNormal)
					v.position.row, v.position.col = v.position.startRow, v.position.startCol
				}
			}
		}

		moveZombies()

		// process collisions
		for _, z := range zombies {
			if player.row == z.position.row && player.col == z.position.col {
				zombiesStatusMx.RLock()
				if z.status == ZombieStatusNormal {
					lives = lives - 1
					if lives != 0 {
						moveCursor(player.row, player.col)
						fmt.Print(cfg.Death)
						moveCursor(len(maze)+2, 0)
						zombiesStatusMx.RUnlock()
						updateZombies(zombies, ZombieStatusNormal)
						time.Sleep(1000 * time.Millisecond) //dramatic pause before reseting player position
						player.row, player.col = player.startRow, player.startCol
					}
				} else if z.status == ZombieStatusBlue {
					zombiesStatusMx.RUnlock()
					updateZombies([]*zombie{z}, ZombieStatusNormal)
					z.position.row, z.position.col = z.position.startRow, z.position.startCol
				}
			}
		}

		// update screen
		printScreen()

		// check game over
		if numDots == 0 || lives <= 0 {
			if lives == 0 {
				moveCursor(player.row, player.col)
				fmt.Print(cfg.Death)
				moveCursor(player.startRow-4, player.startCol-3)
				fmt.Print("╔══════════╗")
				moveCursor(player.startRow-3, player.startCol-2)
				fmt.Print("GAME OVER")
				moveCursor(player.startRow-2, player.startCol-3)
				fmt.Print("╚══════════╝")
				moveCursor(len(maze)+4, 0)
			}

			if numDots == 0 && lives != 0 {
				moveCursor(player.row, player.col)
				fmt.Print(cfg.Death)
				moveCursor(player.startRow-4, player.startCol-3)
				fmt.Print("╔══════════╗")
				moveCursor(player.startRow-3, player.startCol-2)
				fmt.Print(" YOU WIN ")
				moveCursor(player.startRow-2, player.startCol-3)
				fmt.Print("╚══════════╝")
				moveCursor(len(maze)+4, 0)
			}
			break
		}

		// repeat
		time.Sleep(200 * time.Millisecond)
	}
}
