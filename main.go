package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	tcell "github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	// Initialize the tview application
	app := tview.NewApplication()

	// Left pane: List of listening ports
	portList := tview.NewList()
	portList.ShowSecondaryText(false)
	portList.SetBorder(true).SetTitle("Listening Ports")
	portList.SetBackgroundColor(tcell.ColorBlack)
	portList.SetMainTextColor(tcell.ColorWhite)
	portList.SetSelectedTextColor(tcell.ColorBlack)
	portList.SetSelectedBackgroundColor(tcell.ColorWhite)

	// Right upper pane: Output of lsof for the selected port
	lsofOutput := tview.NewTextView()
	lsofOutput.SetDynamicColors(true)
	lsofOutput.SetBorder(true).SetTitle("lsof Output")
	lsofOutput.SetBackgroundColor(tcell.ColorBlack)

	// Right lower pane: Docker ps output if applicable
	dockerOutput := tview.NewTextView().SetDynamicColors(true)
	dockerOutput.SetBorder(true).SetTitle("Docker Container Info")
	dockerOutput.SetBackgroundColor(tcell.ColorBlack)

	// Layout: Flexbox with left list and stacked right panes
	flex := tview.NewFlex()
	flex.SetBackgroundColor(tcell.ColorBlack)
	flex.AddItem(portList, 0, 1, true). // Left pane takes 1/3 of width
						AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
							AddItem(lsofOutput, 0, 1, false).   // Upper right pane
							AddItem(dockerOutput, 0, 1, false), // Lower right pane
			0, 2, false) // Right side takes 2/3 of width

	// Function to retrieve all listening ports, sorted numerically
	getPorts := func() []string {
		cmd := exec.Command("lsof", "-iTCP", "-sTCP:LISTEN")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return []string{fmt.Sprintf("Error fetching ports: %v", err)}
		}
		lines := strings.Split(out.String(), "\n")
		ports := make(map[string]struct{})
		for _, line := range lines {
			if strings.Contains(line, "*") { // Listening on all interfaces
				parts := strings.Fields(line)
				if len(parts) > 8 {
					port := strings.Split(parts[8], ":")[1]
					ports[port] = struct{}{}
				}
			}
		}
		var portSlice []string
		for port := range ports {
			portSlice = append(portSlice, port)
		}
		sort.Slice(portSlice, func(i, j int) bool {
			a, _ := strconv.Atoi(portSlice[i])
			b, _ := strconv.Atoi(portSlice[j])
			return a < b
		})
		return portSlice
	}

	// Function to get lsof output for a specific port
	getLsofOutput := func(port string) string {
		cmd := exec.Command("lsof", "-i", ":"+port)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return out.String()
	}

	// Function to check if the process is Docker-related
	isDockerProcess := func(lsofOut string) bool {
		return strings.Contains(strings.ToLower(lsofOut), "docker")
	}

	// Function to get Docker container info for a specific port
	getDockerInfo := func(port string) string {
		cmd := exec.Command("docker", "ps", "-a", "--filter", "publish="+port)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return out.String()
	}

	// Function to strip ANSI color codes from text
	stripAnsiCodes := func(str string) string {
		re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		return re.ReplaceAllString(str, "")
	}

	// Populate the port list initially
	ports := getPorts()
	portList.AddItem("", "", 0, nil)
	for _, port := range ports {
		portList.AddItem(port, "", 0, nil)
	}

	// Handle selection changes in the port list
	portList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		lsofOut := getLsofOutput(mainText)
		lsofOutput.SetText(lsofOut)
		if isDockerProcess(lsofOut) {
			dockerInfo := getDockerInfo(mainText)
			dockerOutput.SetText(dockerInfo)
		} else {
			dockerOutput.SetText("Not a Docker process")
		}
	})
	portList.SetCurrentItem(1)

	// Set up key bindings for navigation and copying
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'h': // Switch focus to the output pane.
				app.SetFocus(portList)
				return nil
			case 'l': // Switch focus to the output pane.
				app.SetFocus(lsofOutput)
				return nil
			case 'j': // Move down in the list
				if app.GetFocus() == portList {
					current := portList.GetCurrentItem()
					if current < portList.GetItemCount()-1 {
						portList.SetCurrentItem(current + 1)
					}
				} else {
					app.SetFocus(dockerOutput)
				}
			case 'k': // Move up in the list
				if app.GetFocus() == portList {
					current := portList.GetCurrentItem()
					if current > 0 {
						portList.SetCurrentItem(current - 1)
					}
				} else {
					app.SetFocus(lsofOutput)
				}
			case 'q': // Quit the application
				app.Stop()
			case 'c': // Copy text of focused text view
				focused := app.GetFocus()
				if tv, ok := focused.(*tview.TextView); ok {
					text := stripAnsiCodes(tv.GetText(false))
					if err := clipboard.WriteAll(text); err != nil {
						showModal(app, flex, fmt.Sprintf("Error copying to clipboard: \n%s", err.Error()), "Clipboard Error")
					} else {
						showModal(app, flex, "Copied to clipboard", "Clipboard")
					}
				}
			}
		}
		return event
	})

	// Start the application
	if err := app.SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}
}

func showModal(app *tview.Application, flex tview.Primitive, text string, title string) {
	// Create a TextView with left-aligned text.
	textView := tview.NewTextView()
	textView.SetTextAlign(tview.AlignLeft)
	textView.SetTextColor(tcell.ColorWhite)
	textView.SetBackgroundColor(tcell.ColorBlack)
	textView.SetBorder(true)
	textView.SetTitle(title)
	textView.SetText(text)

	// Center the TextView horizontally and vertically by nesting two Flex layouts.
	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false). // Top spacer.
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).      // Left spacer.
			AddItem(textView, 65, 1, true). // The help box (80 columns wide; adjust as needed).
			AddItem(nil, 0, 1, false),      // Right spacer.
						0, 2, true).
		AddItem(nil, 0, 1, false) // Bottom spacer.

	app.SetRoot(modal, true).SetFocus(textView)
	textView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Dismiss the help modal on any key press.
		app.SetRoot(flex, true)
		return nil
	})
}
