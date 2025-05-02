package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// TaskCommand represents the command to run task
type TaskCommand struct {
	Cmd  string
	Args []string
}

// Task represents a task from the Taskfile
type Task struct {
	Name string
	Desc string
	Cmds []string // Added field for commands
}

// Implement list.Item interface
func (t Task) Title() string       { return t.Name }
func (t Task) Description() string { return t.Desc }
func (t Task) FilterValue() string { return t.Name }

var (
	taskCmd TaskCommand
	tasks   []Task
)

// Model represents the TUI state
type model struct {
	list         list.Model
	filter       textinput.Model
	filteredList []list.Item
	allItems     []list.Item
	selected     bool
	err          error
	width        int
	height       int
	showDesc     bool // Whether to show descriptions
	showCmds     bool // Whether to show commands
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gt [task_name] [flags]",
	Short: "Interactive wrapper for Go Task",
	Long: `An interactive CLI wrapper for Go Task that provides a 
fuzzy-searchable interface for your Taskfile tasks.

When run without arguments, launches an interactive TUI.
When run with arguments, passes them directly to task.

Examples:
  gt                  # Launch interactive TUI
  gt build            # Run the 'build' task
  gt -l               # List all available tasks
  gt clean test       # Run 'clean' and then 'test' tasks
`,
	// We don't want cobra's argument validation since we're passing everything to task
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// If args are provided, pass them directly to task
		if len(args) > 0 {
			os.Exit(runTaskDirect(args))
			return
		}

		// Otherwise, start the TUI
		launchTUI()
	},
}

func main() {
	cobra.OnInitialize(initialize)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// initialize runs before command execution
func initialize() {
	var err error

	// Check if task is available
	taskCmd, err = findTaskCommand()
	if err != nil {
		fmt.Println("Error: Task is not installed")
		fmt.Println("Please install Go Task:")
		fmt.Println("- Official repository: https://github.com/go-task/task")
		fmt.Println("- Installation guide: https://taskfile.dev/installation/")
		os.Exit(1)
	}

	// Parse Taskfile
	tasks, err = parseTaskfile()
	if err != nil {
		fmt.Printf("Error parsing Taskfile: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found in Taskfile. Please make sure your Taskfile has tasks defined.")
		os.Exit(1)
	}
}

// findTaskCommand checks if 'task' is available, falls back to 'go tool task',
// and returns the appropriate command or an error if neither is found
func findTaskCommand() (TaskCommand, error) {
	// Check if 'task' is in PATH
	if _, err := exec.LookPath("task"); err == nil {
		return TaskCommand{Cmd: "task", Args: []string{}}, nil
	}

	// Check if 'go tool task' is available
	cmd := exec.Command("go", "tool", "task", "--help")
	if err := cmd.Run(); err == nil {
		return TaskCommand{Cmd: "go", Args: []string{"tool", "task"}}, nil
	}

	// Neither is available
	return TaskCommand{}, fmt.Errorf("task command not found in PATH")
}

// parseTaskfile reads the Taskfile.yml and extracts tasks
func parseTaskfile() ([]Task, error) {
	// Look for Taskfile.yml or Taskfile.yaml in the current directory
	var taskfilePath string
	for _, name := range []string{"Taskfile.yml", "Taskfile.yaml"} {
		if _, err := os.Stat(name); err == nil {
			taskfilePath = name
			break
		}
	}

	if taskfilePath == "" {
		// Look for Taskfile.yml or Taskfile.yaml in parent directories
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		for {
			for _, name := range []string{"Taskfile.yml", "Taskfile.yaml"} {
				path := filepath.Join(dir, name)
				if _, err := os.Stat(path); err == nil {
					taskfilePath = path
					break
				}
			}

			if taskfilePath != "" || dir == "/" {
				break
			}

			// Move to parent directory
			dir = filepath.Dir(dir)
		}
	}

	if taskfilePath == "" {
		return nil, fmt.Errorf("no Taskfile.yml or Taskfile.yaml found")
	}

	data, err := os.ReadFile(taskfilePath)
	if err != nil {
		return nil, err
	}

	// Parse YAML
	var taskfile map[string]interface{}
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		return nil, err
	}

	// Extract tasks
	tasks := []Task{}
	if tasksMap, ok := taskfile["tasks"].(map[string]interface{}); ok {
		for name, details := range tasksMap {
			description := ""
			var commands []string

			if taskDetails, ok := details.(map[string]interface{}); ok {
				// Get description
				if desc, ok := taskDetails["desc"].(string); ok {
					description = desc
				} else if desc, ok := taskDetails["summary"].(string); ok {
					description = desc
				}

				// Get commands
				if cmds, ok := taskDetails["cmds"].([]interface{}); ok {
					for _, cmd := range cmds {
						if cmdStr, ok := cmd.(string); ok {
							commands = append(commands, cmdStr)
						}
					}
				}
			}

			tasks = append(tasks, Task{Name: name, Desc: description, Cmds: commands})
		}
	}

	return tasks, nil
}

// fuzzyFilter filters the list items based on the input
func fuzzyFilter(items []list.Item, filter string) []list.Item {
	if filter == "" {
		return items
	}

	// Extract the string values to match against
	var targets []string
	for _, item := range items {
		targets = append(targets, item.FilterValue())
	}

	// Perform fuzzy matching
	matches := fuzzy.Find(filter, targets)

	// Create a new slice with the matching items in order
	var filtered []list.Item
	for _, match := range matches {
		filtered = append(filtered, items[match.Index])
	}

	return filtered
}

// launchTUI starts the Bubble Tea TUI
func launchTUI() {
	// Convert tasks to list items
	var items []list.Item
	for _, task := range tasks {
		items = append(items, task)
	}

	// Create filter input
	ti := textinput.New()
	ti.Placeholder = "Filter tasks..."
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 30

	// Create list
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("170"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("240"))

	// Reduce spacing between items to absolute minimum
	delegate.SetSpacing(0) // Minimal spacing between items
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.PaddingTop(0).PaddingBottom(0).MarginTop(0).MarginBottom(0)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.PaddingTop(0).PaddingBottom(0).MarginTop(0).MarginBottom(0)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.PaddingTop(0).PaddingBottom(0).MarginTop(0).MarginBottom(0)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.PaddingTop(0).PaddingBottom(0).MarginTop(0).MarginBottom(0)

	l := list.New(items, delegate, 0, 0)
	l.SetShowTitle(false) // Remove the title completely
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // We'll handle filtering ourselves

	// Create initial model
	m := model{
		list:         l,
		filter:       ti,
		filteredList: items,
		allItems:     items,
		showDesc:     false, // Start with descriptions hidden
		showCmds:     false, // Start with commands hidden
	}

	// We won't actually use the filter's focus state anymore
	// but we'll set this to simplify the code
	m.filter.Focus()

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}

// Init initializes the TUI model
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles TUI events
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// First check if filter is focused
		if m.filter.Focused() {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Blur the filter on ESC to enter navigation mode
				m.filter.Blur()
				return m, nil
			case "enter":
				if len(m.filteredList) > 0 {
					i := m.list.SelectedItem()
					task, ok := i.(Task)
					if ok {
						m.selected = true
						// Run the selected task and quit when done
						return m, tea.Sequence(
							tea.ExecProcess(
								exec.Command(taskCmd.Cmd, append(taskCmd.Args, task.Name)...),
								func(err error) tea.Msg {
									return nil
								},
							),
							tea.Quit,
						)
					}
				}
			case "down", "up":
				// Pass navigation keys to the list
				var listCmd tea.Cmd
				m.list, listCmd = m.list.Update(msg)
				cmds = append(cmds, listCmd)
			default:
				// All other keys go to filter input
				var filterCmd tea.Cmd
				m.filter, filterCmd = m.filter.Update(msg)
				cmds = append(cmds, filterCmd)

				// Filter the list based on input
				m.filteredList = fuzzyFilter(m.allItems, m.filter.Value())
				m.list.SetItems(m.filteredList)
			}
		} else {
			// Navigation mode (filter not focused)
			switch msg.String() {
			case "ctrl+c", "esc", "q":
				return m, tea.Quit
			case "enter":
				if len(m.filteredList) > 0 {
					i := m.list.SelectedItem()
					task, ok := i.(Task)
					if ok {
						m.selected = true
						// Run the selected task and quit when done
						return m, tea.Sequence(
							tea.ExecProcess(
								exec.Command(taskCmd.Cmd, append(taskCmd.Args, task.Name)...),
								func(err error) tea.Msg {
									return nil
								},
							),
							tea.Quit,
						)
					}
				}
			case "right", "l":
				// Toggle display modes with right arrow:
				// No desc -> Show desc -> Show desc+cmds -> No desc
				if !m.showDesc {
					// First right arrow: show descriptions
					m.showDesc = true
					m.showCmds = false
				} else if !m.showCmds {
					// Second right arrow: show commands too
					m.showCmds = true
				} else {
					// Third right arrow: back to no extras
					m.showDesc = false
					m.showCmds = false
				}
			case "left", "h":
				// Left arrow always hides everything
				m.showDesc = false
				m.showCmds = false
			case "down", "j":
				// Down navigation
				var listCmd tea.Cmd
				m.list, listCmd = m.list.Update(tea.KeyMsg{Type: tea.KeyDown})
				cmds = append(cmds, listCmd)
			case "up", "k":
				// Up navigation
				var listCmd tea.Cmd
				m.list, listCmd = m.list.Update(tea.KeyMsg{Type: tea.KeyUp})
				cmds = append(cmds, listCmd)
			case "/":
				// Focus the filter input
				m.filter.Focus()
				return m, textinput.Blink
			default:
				// Any other character starts filter and adds it
				m.filter.Focus()
				m.filter.SetValue(msg.String())
				m.filteredList = fuzzyFilter(m.allItems, m.filter.Value())
				m.list.SetItems(m.filteredList)
				return m, textinput.Blink
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-6) // Reserve space for filter and help text
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI
func (m model) View() string {
	if m.selected {
		return "Running task..."
	}

	// Create a clean filter without border
	filterStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Width(m.width - 4)

	// Simple filter display - no mode indicators
	var filterContent string
	if m.filter.Value() == "" {
		filterContent = "Type to filter tasks..."
	} else {
		filterContent = "Filter: " + m.filter.Value()
	}

	filterView := filterStyle.Render(filterContent)

	// Create a custom ultra-compact list rendering
	var listItems strings.Builder

	selected := m.list.Index()
	for i, item := range m.filteredList {
		task := item.(Task)

		// Apply styling based on selection state
		var lineStyle lipgloss.Style
		if i == selected {
			lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
		} else {
			lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		}

		// Render line with task name only by default
		line := task.Name

		// Add description if enabled for selected item
		if m.showDesc && i == selected && task.Desc != "" {
			line += " - " + task.Desc
		}

		// Add commands if enabled for selected item
		if m.showCmds && i == selected && len(task.Cmds) > 0 {
			line += "\n  cmds:"
			for _, cmd := range task.Cmds {
				line += "\n    - " + cmd
			}
		}

		listItems.WriteString(lineStyle.Render(line) + "\n")
	}

	// Simple help text without mode indicators
	helpText := "\n↑/↓: navigate • →: toggle details • ←: hide details • enter: select • q: quit"

	return "\n" + filterView + "\n\n" + listItems.String() + helpText
}

// runTaskDirect passes args directly to task command
func runTaskDirect(args []string) int {
	// Create the combined args
	fullArgs := append(taskCmd.Args, args...)

	// Create and run command
	cmd := exec.Command(taskCmd.Cmd, fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Run the command and return the exit code
	err := cmd.Run()
	if err != nil {
		// Check if it's an exit error to get the exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		// Other error occurred
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}
