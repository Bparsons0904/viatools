package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	cursor  int
	choices []string
	step    string
}

func init() {
	fmt.Print("\033[H\033[2J")
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

	m.step = "downloading"
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
		m.step = "confirmDeleteFolder"
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
	var updatedChoices []string
	for _, choice := range m.choices {
		if choice == "Download Latest" {
			choice = "Check Stage Download Status"
			updatedChoices = append(updatedChoices, choice)
		} else {
			updatedChoices = append(updatedChoices, choice)
		}
	}

	m.choices = updatedChoices
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.step == "confirmDeleteFolder" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y":
				deleteFolder(&m)
			case "n":
				m.step = ""
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
				m.step = "confirmDeleteFolder"
			case 1:
				log.Println("Load Stage")
			}
		}
	}

	return m, nil
}

var (
	pinkColor   = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Align(lipgloss.Left)
	blueColor   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Align(lipgloss.Left)
	noticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true).Align(lipgloss.Left)
	whiteColor  = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Align(lipgloss.Left)
	mainStyle   = lipgloss.NewStyle()
	redColor    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Align(lipgloss.Left)
	greenColor  = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Align(lipgloss.Left)
)

func (m model) View() string {
	s := ""

	if m.step == "confirmDeleteFolder" {
		s += fmt.Sprintf("Are you sure you want to delete the existing folder? (%s/%s)\n", greenColor.Render("y"), redColor.Render("n"))
		return s
	}

	if m.step == "downloading" {
		s += fmt.Sprintf("%s\n%s\n%s\n%s\n\n\n",
			greenColor.Render("Starting download!"),
			blueColor.Render("This will take a while. Please wait..."),
			blueColor.Render("You can check the status by running"),
			pinkColor.Render("`tmux a -t download-latest-session`"),
		)
	}

	s += fmt.Sprintf("%s\n\n", noticeStyle.Render("What should we do today?"))

	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		choiceStyle := whiteColor.Copy()
		if m.cursor == i {
			choiceStyle = blueColor.Copy()
		}

		s += fmt.Sprintf("%s %s\n", cursor, choiceStyle.Render(choice))
	}

	s += fmt.Sprintf("\n%s\n%s\n", whiteColor.Render("Use the arrow keys to navigate. Press Enter to select."), blueColor.Render("Press q to <C-c> to exit"))

	return mainStyle.Render(s)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "Program panicked:", r)
		}
		resetTerminal()
	}()

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

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
