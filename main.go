package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	cursor              int
	choices             []string
	confirmDeleteFolder bool
}

func initialModel() model {
	return model{
		choices: []string{"Download Latest", "Load Stage"},
	}
}

func (m model) Init() tea.Cmd {
	return tea.SetWindowTitle("VIA Tools")
}

type Command struct {
	message string
	args    []string
	envs    []string
}

func runBashCommand(command Command) {
	cmd := exec.Command(command.message, command.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if len(command.envs) > 0 {
		cmd.Env = append(os.Environ(), command.envs...)
	}

	err := cmd.Run()
	if err != nil {
		log.Fatalf("Command execution failed with error: %v", err)
	}
}

func deleteFolder(m *model) {
	pathToFolder := os.Getenv("VIA_STAGE_FILE_PATH")
	if pathToFolder == "" {
		log.Fatal("VIA_STAGE_FILE_PATH must be set")
	}
	deleteFolderCmd := []string{"-rf", pathToFolder}
	cmd := Command{
		message: "rm",
		args:    deleteFolderCmd,
		envs:    []string{},
	}
	runBashCommand(cmd)

	m.confirmDeleteFolder = false
	downloadLatest(m)
}

func downloadLatest(m *model) {
	pathToFolder := os.Getenv("VIA_STAGE_FILE_PATH")
	password := os.Getenv("VIA_STAGE_PASSWORD")

	if password == "" || pathToFolder == "" {
		log.Fatal("VIA_STAGE_PASSWORD and VIA_STAGE_FILE_PATH must be set")
	}

	_, err := os.Stat(pathToFolder)
	if !os.IsNotExist(err) {
		m.confirmDeleteFolder = true
		return
	}

	sessionName := "download-latest-session"

	pgDumpCmd := fmt.Sprintf(
		"pg_dump -h localhost -p 2234 -U stage-crm-backend -F d -j 4 -Z 4 -f %s stage",
		pathToFolder,
	)

	startSessionCmd := []string{
		"new-session", "-d", "-s", sessionName, "sh", "-c",
		pgDumpCmd,
	}

	cmd := Command{
		message: "tmux",
		args:    startSessionCmd,
		envs:    []string{"PGPASSWORD=" + password},
	}

	runBashCommand(cmd)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.confirmDeleteFolder {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y":
				deleteFolder(&m)
				m.confirmDeleteFolder = false
			case "n":
				m.confirmDeleteFolder = false
			}
		}

		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			switch m.cursor {
			case 0:
				downloadLatest(&m)
			case 1:
				log.Println("Load Stage")
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.confirmDeleteFolder {
		return "Are you sure you want to delete the existing folder? (y/n)\n"
		// return "\033[H\033[2J" + "Are you sure you want to delete the existing folder? (y/n)\n"
	}

	s := "\033[H\033[2J" + "What should we do today?\n\n"

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		s += fmt.Sprintf("%s %s\n", cursor, choice)
	}

	s += "\nPress q to quit.\n"

	return s
}

func main() {
	// Handle terminal reset on exit
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "Program panicked:", r)
		}
		resetTerminal()
	}()

	// Capture SIGINT and SIGTERM signals to reset terminal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		resetTerminal()
		os.Exit(0)
	}()

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		resetTerminal()
		os.Exit(1)
	}
}

func resetTerminal() {
	if isTerminal(os.Stdin) {
		cmd := exec.Command("stty", "sane")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}

// isTerminal checks if the given file descriptor is a terminal
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
