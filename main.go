package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type WeatherData struct {
	PlaceName     string       `json:"place_name"`
	PlaceID       string       `json:"place_id"`
	PrefecturesID string       `json:"prefectures_id"`
	DateTime      string       `json:"dateTime"`
	Yesterday     []HourlyData `json:"yesterday"`
	Today         []HourlyData `json:"today"`
	Tomorrow      []HourlyData `json:"tomorrow"`
	DayAfterTom   []HourlyData `json:"dayaftertomorrow"`
}

type HourlyData struct {
	Time          string `json:"time"`
	Weather       string `json:"weather"`
	Temp          string `json:"temp"`
	Pressure      string `json:"pressure"`
	PressureLevel string `json:"pressure_level"`
}

type model struct {
	weatherData WeatherData
	dayFilter   string
	areaCode    string
	loading     bool
	err         error
	scrollPos   int // Current scroll position for content pagination
	currentDay  int // Current day index for horizontal pagination (0=Yesterday, 1=Today, 2=Tomorrow, 3=DayAfter)
	width       int // Terminal width
	height      int // Terminal height
}

var (
	// Main application style with a visible rounded border frame without background
	// This ensures the border is visible while maximizing space for content
	appStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#003366"))

	dayHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#0A4B78")).
			PaddingLeft(1).
			PaddingRight(1).
			MarginTop(1).
			MarginBottom(1).
			Width(58).
			Align(lipgloss.Center)

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#1A659E")).
				PaddingLeft(1).
				PaddingRight(1).
				Align(lipgloss.Center)

	cellStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1).
			Align(lipgloss.Center)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true).
			Padding(1)

	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4D94FF")).
			Bold(true).
			Padding(2).
			Align(lipgloss.Center)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7FDBFF")).
			Padding(1, 0).
			Align(lipgloss.Center)
)

// parseFloat converts a string to float64, returning 0 if conversion fails
func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

func translateWeatherCode(code string) string {
	switch code {
	case "100":
		return "Sunny"
	case "200":
		return "Cloudy"
	case "300":
		return "Rainy"
	default:
		return "Unknown"
	}
}

// Column widths for table rendering
const (
	timeWidth     = 8
	weatherWidth  = 10
	tempWidth     = 10
	pressureWidth = 15
	levelWidth    = 20
)

// formatHourlyData formats a single hourly data entry for display
func formatHourlyData(entry HourlyData) (string, string, string, string) {
	temp := entry.Temp
	if temp == "#" {
		temp = "N/A"
	}
	pressure := entry.Pressure
	if pressure == "#" {
		pressure = "N/A"
	}

	// Format hour to two digits
	hour := strings.TrimSpace(entry.Time)
	if len(hour) == 1 {
		hour = "0" + hour
	}

	// Format temperature to one decimal place if it's not N/A
	if temp != "N/A" {
		temp = fmt.Sprintf("%.1f", parseFloat(temp))
	}

	// Format pressure to one decimal place if it's not N/A
	if pressure != "N/A" {
		pressure = strings.TrimSpace(pressure)
		pressure = fmt.Sprintf("%.1f", parseFloat(pressure))
	}

	weatherDesc := translateWeatherCode(entry.Weather)

	return hour + ":00", weatherDesc, temp, pressure
}

// createTableHeaders creates the table headers for the weather data
func createTableHeaders() string {
	tableHeader := tableHeaderStyle.Width(timeWidth).Render("Time") +
		tableHeaderStyle.Width(weatherWidth).Render("Weather") +
		tableHeaderStyle.Width(tempWidth).Render("Temp") +
		tableHeaderStyle.Width(pressureWidth).Render("Pressure") +
		tableHeaderStyle.Width(levelWidth).Render("Pressure Level")

	tableUnits := tableHeaderStyle.Width(timeWidth).Render("") +
		tableHeaderStyle.Width(weatherWidth).Render("") +
		tableHeaderStyle.Width(tempWidth).Render("(°C)") +
		tableHeaderStyle.Width(pressureWidth).Render("(hPa)") +
		tableHeaderStyle.Width(levelWidth).Render("")

	return tableHeader + "\n" + tableUnits
}

// calculateScrollParameters calculates the visible height and maximum scroll position
func calculateScrollParameters(m model, numHeaders int, numContentLines int) (int, int) {
	// Define how many lines can be displayed at once (excluding headers)
	// Use the actual terminal height from the model
	headerLines := numHeaders * 3 // 3 lines per day header
	// Account for app padding (2 lines), scroll indicator spacing (2 lines), and footer (3 lines)
	extraLines := 7
	// Calculate available height for content
	visibleHeight := m.height - headerLines - extraLines
	if visibleHeight < 3 {
		visibleHeight = 3 // Ensure at least some content is visible
	}

	// Calculate the maximum possible scroll position
	maxScroll := numContentLines - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
		// When content is shorter than visible height, show all content
		visibleHeight = numContentLines
	}

	return visibleHeight, maxScroll
}

// getDayData returns the day name and data for a given day index
// 0=Yesterday, 1=Today, 2=Tomorrow, 3=DayAfterTomorrow
func (m model) getDayData(dayIndex int) (string, []HourlyData) {
	var dayName string
	var dayData []HourlyData

	switch dayIndex {
	case 0:
		dayName = "Yesterday"
		dayData = m.weatherData.Yesterday
	case 1:
		dayName = "Today"
		dayData = m.weatherData.Today
	case 2:
		dayName = "Tomorrow"
		dayData = m.weatherData.Tomorrow
	case 3:
		dayName = "Day After Tomorrow"
		dayData = m.weatherData.DayAfterTom
	default:
		dayName = "Today"
		dayData = m.weatherData.Today
	}

	return dayName, dayData
}

// extractHeadersAndContent extracts headers and content for a day's data
func (m model) extractHeadersAndContent(dayName string, data []HourlyData) (string, string) {
	if len(data) == 0 {
		return "", ""
	}

	// Create day header with place name
	dayHeader := dayHeaderStyle.Render(fmt.Sprintf("%s - %s", m.weatherData.PlaceName, dayName))

	// Create table headers
	tableHeaders := createTableHeaders()

	// Create the combined header
	headers := dayHeader + "\n" + tableHeaders

	// Create data rows
	var rows []string
	for _, entry := range data {
		hour, weather, temp, pressure := formatHourlyData(entry)

		row := cellStyle.Width(timeWidth).Render(hour) +
			cellStyle.Width(weatherWidth).Render(weather) +
			cellStyle.Width(tempWidth).Render(temp) +
			cellStyle.Width(pressureWidth).Render(pressure) +
			cellStyle.Width(levelWidth).Render(entry.PressureLevel)

		rows = append(rows, row)
	}

	// Join rows into content
	content := strings.Join(rows, "\n")

	return headers, content
}

func (m model) View() string {
	// Handle error and loading states
	if m.err != nil {
		content := errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
		return appStyle.Render(content)
	}

	if m.loading {
		content := loadingStyle.Render("Loading weather data...\nPlease wait")
		return appStyle.Render(content)
	}

	// Process data based on day filter
	var allHeaders []string
	var allContent strings.Builder

	switch strings.ToLower(m.dayFilter) {
	case "yesterday":
		headers, content := m.extractHeadersAndContent("Yesterday", m.weatherData.Yesterday)
		allHeaders = append(allHeaders, headers)
		allContent.WriteString(content)
	case "today":
		headers, content := m.extractHeadersAndContent("Today", m.weatherData.Today)
		allHeaders = append(allHeaders, headers)
		allContent.WriteString(content)
	case "tomorrow":
		headers, content := m.extractHeadersAndContent("Tomorrow", m.weatherData.Tomorrow)
		allHeaders = append(allHeaders, headers)
		allContent.WriteString(content)
	case "dayafter":
		headers, content := m.extractHeadersAndContent("Day After Tomorrow", m.weatherData.DayAfterTom)
		allHeaders = append(allHeaders, headers)
		allContent.WriteString(content)
	case "":
		// If no day specified, show only the current day based on horizontal pagination
		dayName, dayData := m.getDayData(m.currentDay)

		if headers, content := m.extractHeadersAndContent(dayName, dayData); headers != "" {
			allHeaders = append(allHeaders, headers)
			allContent.WriteString(content)
		}
	default:
		allContent.WriteString(errorStyle.Render("Invalid day specified. Please use: yesterday, today, tomorrow, or dayafter"))
	}

	// Split the content into lines for scrolling (excluding headers)
	contentLines := strings.Split(allContent.String(), "\n")

	// Calculate scroll parameters
	visibleHeight, maxScroll := calculateScrollParameters(m, len(allHeaders), len(contentLines))

	// Ensure scroll position stays within valid bounds
	if m.scrollPos < 0 {
		m.scrollPos = 0
	} else if m.scrollPos > maxScroll {
		m.scrollPos = maxScroll
	}

	// Determine start position for content
	// Always apply scroll position regardless of terminal size
	// This ensures content is compressed rather than headers being hidden
	startPos := m.scrollPos

	// Ensure we don't go out of bounds
	if startPos >= len(contentLines) && len(contentLines) > 0 {
		startPos = len(contentLines) - 1
	}

	// Extract only the lines that should be visible based on scroll position
	endIdx := startPos + visibleHeight
	if endIdx > len(contentLines) {
		endIdx = len(contentLines)
	}

	var visibleContentLines []string
	if len(contentLines) > 0 && contentLines[0] != "" {
		visibleContentLines = contentLines[startPos:endIdx]
	}

	// Join the visible content lines
	visibleContent := strings.Join(visibleContentLines, "\n")

	// Create scroll indicators to show users there's more content
	var scrollIndicator string
	if m.scrollPos > 0 && maxScroll > 0 {
		scrollIndicator += "↑ More above"
	}

	// Add scroll down indicator if there's more content below
	if m.scrollPos < maxScroll {
		if scrollIndicator != "" {
			scrollIndicator += " | "
		}
		scrollIndicator += "↓ More below"
	}

	// Combine headers with scrollable content
	var finalContent strings.Builder

	// Create a separate builder for content that will be added after the frame is rendered
	var contentBuilder strings.Builder

	// Add scroll indicator if needed
	if scrollIndicator != "" {
		contentBuilder.WriteString(scrollIndicator + "\n\n")
	}

	// Add all day headers
	for i, header := range allHeaders {
		contentBuilder.WriteString(header + "\n")

		// If this is the current day being viewed, add the scrollable content
		if i == len(allHeaders)-1 {
			if visibleContent != "" {
				contentBuilder.WriteString(visibleContent)
			}
		}

		// Add spacing between days
		if i < len(allHeaders)-1 {
			contentBuilder.WriteString("\n\n")
		}
	}

	// Footer with navigation help for users
	var footerText string
	if m.dayFilter == "" {
		// Show left/right navigation instructions when no day filter is set
		footerText = "←/→: Change day  ↑/↓/Mouse wheel: Scroll  PgUp/PgDn: Scroll faster  Home/End: Jump to top/bottom  q: Quit"
	} else {
		footerText = "↑/↓/Mouse wheel: Scroll  PgUp/PgDn: Scroll faster  Home/End: Jump to top/bottom  q: Quit"
	}
	footer := footerStyle.Render(footerText)

	// Add the footer
	contentBuilder.WriteString("\n\n" + footer)

	// Add the content to the final output
	finalContent.WriteString(contentBuilder.String())

	// Render the frame with the content
	return appStyle.Render(finalContent.String())
}

// safeGetString safely extracts a string value from a map
func safeGetString(data map[string]interface{}, key string) string {
	if value, exists := data[key]; exists {
		return fmt.Sprintf("%v", value)
	}
	return ""
}

// parseHourlyData parses hourly weather data from the API response
func parseHourlyData(data interface{}) []HourlyData {
	var result []HourlyData

	hourlyArray, ok := data.([]interface{})
	if !ok {
		return result
	}

	for _, item := range hourlyArray {
		hourlyMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		entry := HourlyData{
			Time:          safeGetString(hourlyMap, "time"),
			Weather:       safeGetString(hourlyMap, "weather"),
			Temp:          safeGetString(hourlyMap, "temp"),
			Pressure:      safeGetString(hourlyMap, "pressure"),
			PressureLevel: safeGetString(hourlyMap, "pressure_level"),
		}
		result = append(result, entry)
	}

	return result
}

// fetchWeatherData fetches weather data from the API
func fetchWeatherData(areaCode string) (WeatherData, error) {
	url := fmt.Sprintf("https://zutool.jp/api/getweatherstatus/%s", areaCode)

	// Make a GET request
	resp, err := http.Get(url)
	if err != nil {
		return WeatherData{}, fmt.Errorf("error making GET request: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("Error closing response body: %v\n", cerr)
		}
	}()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return WeatherData{}, fmt.Errorf("error reading response body: %v", err)
	}

	// Parse JSON data into a generic map
	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return WeatherData{}, fmt.Errorf("error parsing JSON: %v", err)
	}

	// Initialize the WeatherData struct with basic fields
	weatherData := WeatherData{
		PlaceName:     safeGetString(rawData, "place_name"),
		PlaceID:       safeGetString(rawData, "place_id"),
		PrefecturesID: safeGetString(rawData, "prefectures_id"),
		DateTime:      safeGetString(rawData, "dateTime"),
	}

	// Parse each day's data
	if yesterday, exists := rawData["yesterday"]; exists {
		weatherData.Yesterday = parseHourlyData(yesterday)
	}

	if today, exists := rawData["today"]; exists {
		weatherData.Today = parseHourlyData(today)
	}

	// Handle both correct and misspelled versions of "tomorrow"
	if tomorrow, exists := rawData["tomorrow"]; exists {
		weatherData.Tomorrow = parseHourlyData(tomorrow)
	} else if tomorrow, exists := rawData["tommorow"]; exists {
		// Handle the misspelled version from the API
		weatherData.Tomorrow = parseHourlyData(tomorrow)
	}

	if dayAfterTom, exists := rawData["dayaftertomorrow"]; exists {
		weatherData.DayAfterTom = parseHourlyData(dayAfterTom)
	}

	return weatherData, nil
}

// initialModel creates the initial model
func initialModel(areaCode, dayFilter string) model {
	// Set the default day index to today
	currentDay := 1

	// If dayFilter is specified, set the currentDay accordingly
	if dayFilter != "" {
		switch strings.ToLower(dayFilter) {
		case "yesterday":
			currentDay = 0
		case "today":
			currentDay = 1
		case "tomorrow":
			currentDay = 2
		case "dayafter":
			currentDay = 3
		}
	}

	m := model{
		dayFilter:  dayFilter,
		areaCode:   areaCode,
		loading:    true,
		scrollPos:  0,
		currentDay: currentDay,
		width:      80,
		height:     24,
	}
	return m
}

// Start the model with a command to fetch weather data
func (m model) Init() tea.Cmd {
	return fetchWeatherCmd(m.areaCode)
}

// fetchWeatherCmd creates a command to fetch weather data
func fetchWeatherCmd(areaCode string) tea.Cmd {
	return func() tea.Msg {
		weatherData, err := fetchWeatherData(areaCode)
		if err != nil {
			return fetchErrorMsg{err}
		}
		return fetchSuccessMsg{weatherData}
	}
}

type fetchSuccessMsg struct {
	weatherData WeatherData
}

type fetchErrorMsg struct {
	err error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update the model with the new window size
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.MouseMsg:
		// Handle mouse wheel for scrolling
		if msg.Button == tea.MouseButtonWheelUp {
			// Scroll up (decrease scroll position)
			if m.scrollPos > 0 {
				m.scrollPos--
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			// Scroll down (increase scroll position)
			m.scrollPos++
			return m, nil
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			// Scroll up (decrease scroll position)
			if m.scrollPos > 0 {
				m.scrollPos--
			}
			return m, nil
		case "down", "j":
			// Scroll down (increase scroll position)

			// Get the current day's data
			_, dayData := m.getDayData(m.currentDay)

			// Create a content lines array based on the data length
			var contentLines []string
			if len(dayData) > 0 {
				contentLines = make([]string, len(dayData))
			}

			// Calculate scroll parameters
			_, maxScroll := calculateScrollParameters(m, 1, len(contentLines))

			// Only increment if we're not at the maximum
			if m.scrollPos < maxScroll {
				m.scrollPos++
			}
			return m, nil
		case "left", "h":
			// Navigate to previous day
			if m.dayFilter == "" {
				// Only navigate between days when no specific day filter is set
				if m.currentDay > 0 {
					m.currentDay--
					m.scrollPos = 0 // Reset scroll position when changing days
				}
			}
			return m, nil
		case "right", "l":
			// Navigate to next day
			if m.dayFilter == "" {
				// Only navigate between days when no specific day filter is set
				if m.currentDay < 3 {
					m.currentDay++
					m.scrollPos = 0 // Reset scroll position when changing days
				}
			}
			return m, nil
		case "home":
			// Scroll to top
			m.scrollPos = 0
			return m, nil
		case "end":
			// Scroll to bottom - this is approximate, will be limited in View
			m.scrollPos = 999
			return m, nil
		case "pageup":
			// Scroll up by 10 lines
			m.scrollPos -= 10
			if m.scrollPos < 0 {
				m.scrollPos = 0
			}
			return m, nil
		case "pagedown":
			// Scroll down by 10 lines
			m.scrollPos += 10
			return m, nil
		}
	case fetchSuccessMsg:
		m.weatherData = msg.weatherData
		m.loading = false
		return m, nil
	case fetchErrorMsg:
		m.err = msg.err
		m.loading = false
		return m, nil
	}
	return m, nil
}

func main() {
	// Create a custom FlagSet
	fs := flag.NewFlagSet("goHeadache", flag.ExitOnError)
	dayFlag := fs.String("day", "", "Filter output by day (yesterday, today, tomorrow, dayafter)")

	// Print usage if no arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage:  goHeadache <area_code> [-day <day>]")
		fmt.Println("\nOptions:")
		fmt.Println("  -day: yesterday, today, tomorrow, or dayafter")
		fmt.Println("\nPlease visit https://geoshape.ex.nii.ac.jp/ka/resource/ to find the appropriate area code.")
		return
	}

	// Find area code and parse flags regardless of order
	var areaCode string
	var args []string

	// Separate area code from flags
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, "-") && areaCode == "" {
			areaCode = arg
		} else {
			args = append(args, arg)
		}
	}

	// Parse remaining arguments as flags
	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error parsing flags: %v\n", err)
		return
	}

	// Validate we have an area code
	if areaCode == "" {
		fmt.Println("Error: Area code is required")
		return
	}

	// Initialize the model and run the program with full-screen mode
	p := tea.NewProgram(
		initialModel(areaCode, *dayFlag),
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run the program with the fetch command
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
