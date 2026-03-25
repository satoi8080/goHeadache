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
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	scrollPos   int
	currentDay  int // 0=Yesterday, 1=Today, 2=Tomorrow, 3=DayAfterTomorrow
	width       int
	height      int
}

var (
	appStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#0EA5E9"))

	dayHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1E3A5F")).
			Background(lipgloss.Color("#93C5FD")).
			PaddingLeft(2).
			PaddingRight(2).
			MarginTop(1).
			MarginBottom(0).
			Align(lipgloss.Center)

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#0C2A4A")).
				Background(lipgloss.Color("#60A5FA")).
				PaddingLeft(1).
				PaddingRight(1).
				Align(lipgloss.Center)

	cellStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color("#1E293B"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#991B1B")).
			Bold(true).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#EF4444"))

	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0369A1")).
			Bold(true).
			Padding(2).
			Align(lipgloss.Center)

	currentCellStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				PaddingRight(1).
				Align(lipgloss.Center).
				Background(lipgloss.Color("#FEF08A")).
				Foreground(lipgloss.Color("#1E293B")).
				Bold(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#475569")).
			Padding(0, 0).
			MarginTop(1).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("#1E3A5F")).
			Align(lipgloss.Center)
)

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

const numCols = 5

func formatHourlyData(entry HourlyData) (string, string, string, string) {
	temp := entry.Temp
	if temp == "#" {
		temp = "N/A"
	}
	pressure := entry.Pressure
	if pressure == "#" {
		pressure = "N/A"
	}

	hour := strings.TrimSpace(entry.Time)
	if len(hour) == 1 {
		hour = "0" + hour
	}

	if temp != "N/A" {
		temp = fmt.Sprintf("%.1f", parseFloat(temp))
	}

	if pressure != "N/A" {
		pressure = fmt.Sprintf("%.1f", parseFloat(strings.TrimSpace(pressure)))
	}

	return hour + ":00", translateWeatherCode(entry.Weather), temp, pressure
}

func createTableHeaders(colW int) string {
	tableHeader := tableHeaderStyle.Width(colW).Render("Time") +
		tableHeaderStyle.Width(colW).Render("Weather") +
		tableHeaderStyle.Width(colW).Render("Temp") +
		tableHeaderStyle.Width(colW).Render("Pressure") +
		tableHeaderStyle.Width(colW).Render("Pressure Level")

	tableUnits := tableHeaderStyle.Width(colW).Render("") +
		tableHeaderStyle.Width(colW).Render("") +
		tableHeaderStyle.Width(colW).Render("(°C)") +
		tableHeaderStyle.Width(colW).Render("(hPa)") +
		tableHeaderStyle.Width(colW).Render("")

	return tableHeader + "\n" + tableUnits
}

func calculateScrollParameters(m model, numHeaders int, numContentLines int) (int, int) {
	// Lines consumed per header:
	//   dayHeader with MarginTop(1)+content+MarginBottom(1) = 3 lines
	//   "\n" separator between dayHeader and tableHeaders    = 1 line
	//   table header row + table units row                   = 2 lines
	//   trailing "\n" written to contentBuilder              = 1 line
	//   Total: 7 lines per header
	headerLines := numHeaders * 7
	// Fixed overhead lines (not headers or content):
	//   appStyle border (top+bottom) + padding (top+bottom)  = 4 lines
	//   scroll indicator text + blank line                   = 2 lines
	//   "\n\n" before footer                                 = 2 lines
	//   footer with Padding(1,0) and 2 content lines         = 4 lines
	//   Total: 12 lines
	extraLines := 12
	visibleHeight := m.height - headerLines - extraLines
	if visibleHeight < 3 {
		visibleHeight = 3
	}

	maxScroll := numContentLines - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
		visibleHeight = numContentLines
	}

	return visibleHeight, maxScroll
}

// getDayData returns the day name and data for a given day index.
func (m model) getDayData(dayIndex int) (string, []HourlyData) {
	switch dayIndex {
	case 0:
		return "Yesterday", m.weatherData.Yesterday
	case 1:
		return "Today", m.weatherData.Today
	case 2:
		return "Tomorrow", m.weatherData.Tomorrow
	case 3:
		return "Day After Tomorrow", m.weatherData.DayAfterTom
	default:
		return "Today", m.weatherData.Today
	}
}

// findCurrentRowIndex returns the index of the latest entry whose hour <= current hour.
func findCurrentRowIndex(data []HourlyData) int {
	now := time.Now().Hour()
	best := 0
	for i, entry := range data {
		h, err := strconv.Atoi(strings.TrimSpace(entry.Time))
		if err != nil {
			continue
		}
		if h <= now {
			best = i
		}
	}
	return best
}

func (m model) extractHeadersAndContent(dayName string, data []HourlyData, highlightRow int) (string, string) {
	if len(data) == 0 {
		return "", ""
	}

	// appStyle has border(1 each side) + padding(2 each side) = 6 chars total horizontal overhead
	colW := (m.width - 6) / numCols
	tableWidth := colW * numCols
	headers := dayHeaderStyle.Width(tableWidth).Render(fmt.Sprintf("%s - %s", m.weatherData.PlaceName, dayName)) +
		"\n" + createTableHeaders(colW)

	rows := make([]string, len(data))
	for i, entry := range data {
		hour, weather, temp, pressure := formatHourlyData(entry)
		s := cellStyle
		if i == highlightRow {
			s = currentCellStyle
		}
		rows[i] = s.Width(colW).Render(hour) +
			s.Width(colW).Render(weather) +
			s.Width(colW).Render(temp) +
			s.Width(colW).Render(pressure) +
			s.Width(colW).Render(entry.PressureLevel)
	}

	return headers, strings.Join(rows, "\n")
}

func newView(content string) tea.View {
	v := tea.NewView(appStyle.Render(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m model) View() tea.View {
	if m.err != nil {
		return newView(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}
	if m.loading {
		return newView(loadingStyle.Render("Loading weather data...\nPlease wait"))
	}

	var allHeaders []string
	var allContent string

	switch strings.ToLower(m.dayFilter) {
	case "", "yesterday", "today", "tomorrow", "dayafter":
		dayName, dayData := m.getDayData(m.currentDay)
		highlightRow := -1
		if m.currentDay == 1 {
			highlightRow = findCurrentRowIndex(dayData)
		}
		if headers, content := m.extractHeadersAndContent(dayName, dayData, highlightRow); headers != "" {
			allHeaders = append(allHeaders, headers)
			allContent = content
		}
	default:
		allContent = errorStyle.Render("Invalid day specified. Please use: yesterday, today, tomorrow, or dayafter")
	}

	contentLines := strings.Split(allContent, "\n")
	visibleHeight, maxScroll := calculateScrollParameters(m, len(allHeaders), len(contentLines))

	if m.scrollPos < 0 {
		m.scrollPos = 0
	} else if m.scrollPos > maxScroll {
		m.scrollPos = maxScroll
	}

	startPos := m.scrollPos
	if startPos >= len(contentLines) && len(contentLines) > 0 {
		startPos = len(contentLines) - 1
	}
	endIdx := startPos + visibleHeight
	if endIdx > len(contentLines) {
		endIdx = len(contentLines)
	}

	var visibleContent string
	if len(contentLines) > 0 && contentLines[0] != "" {
		visibleContent = strings.Join(contentLines[startPos:endIdx], "\n")
	}

	var indicatorParts []string
	if m.scrollPos > 0 && maxScroll > 0 {
		indicatorParts = append(indicatorParts, "↑ More above")
	}
	if m.scrollPos < maxScroll {
		indicatorParts = append(indicatorParts, "↓ More below")
	}

	var b strings.Builder
	if len(indicatorParts) > 0 {
		b.WriteString(strings.Join(indicatorParts, " | ") + "\n\n")
	}
	for i, header := range allHeaders {
		b.WriteString(header + "\n")
		if i == len(allHeaders)-1 && visibleContent != "" {
			b.WriteString(visibleContent)
		}
		if i < len(allHeaders)-1 {
			b.WriteString("\n\n")
		}
	}

	var footerText string
	if m.dayFilter == "" {
		footerText = "←/→: Change day ↑/↓/Mouse wheel: Scroll \n PgUp/PgDn: Scroll faster  Home/End: Jump to top/bottom  q: Quit"
	} else {
		footerText = "↑/↓/Mouse wheel: Scroll PgUp/PgDn: Scroll faster \n Home/End: Jump to top/bottom  q: Quit"
	}
	tableWidth := ((m.width - 6) / numCols) * numCols
	b.WriteString("\n" + footerStyle.Width(tableWidth).Render(footerText))

	return newView(b.String())
}

func safeGetString(data map[string]interface{}, key string) string {
	if value, exists := data[key]; exists {
		return fmt.Sprintf("%v", value)
	}
	return ""
}

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
		result = append(result, HourlyData{
			Time:          safeGetString(hourlyMap, "time"),
			Weather:       safeGetString(hourlyMap, "weather"),
			Temp:          safeGetString(hourlyMap, "temp"),
			Pressure:      safeGetString(hourlyMap, "pressure"),
			PressureLevel: safeGetString(hourlyMap, "pressure_level"),
		})
	}

	return result
}

func fetchWeatherData(areaCode string) (WeatherData, error) {
	url := fmt.Sprintf("https://zutool.jp/api/getweatherstatus/%s", areaCode)

	resp, err := http.Get(url)
	if err != nil {
		return WeatherData{}, fmt.Errorf("error making GET request: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("Error closing response body: %v\n", cerr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return WeatherData{}, fmt.Errorf("error reading response body: %v", err)
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return WeatherData{}, fmt.Errorf("error parsing JSON: %v", err)
	}

	weatherData := WeatherData{
		PlaceName:     safeGetString(rawData, "place_name"),
		PlaceID:       safeGetString(rawData, "place_id"),
		PrefecturesID: safeGetString(rawData, "prefectures_id"),
		DateTime:      safeGetString(rawData, "dateTime"),
	}

	if yesterday, exists := rawData["yesterday"]; exists {
		weatherData.Yesterday = parseHourlyData(yesterday)
	}
	if today, exists := rawData["today"]; exists {
		weatherData.Today = parseHourlyData(today)
	}
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

func initialModel(areaCode, dayFilter string) model {
	currentDay := 1
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

	return model{
		dayFilter:  dayFilter,
		areaCode:   areaCode,
		loading:    true,
		currentDay: currentDay,
		width:      80,
		height:     24,
	}
}

// Init starts the model with a command to fetch weather data.
func (m model) Init() tea.Cmd {
	return fetchWeatherCmd(m.areaCode)
}

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

func (m model) maxScroll() int {
	_, dayData := m.getDayData(m.currentDay)
	_, maxPos := calculateScrollParameters(m, 1, len(dayData))
	return maxPos
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			if m.scrollPos > 0 {
				m.scrollPos--
			}
		case tea.MouseWheelDown:
			m.scrollPos++
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.scrollPos > 0 {
				m.scrollPos--
			}
		case "down", "j":
			if m.scrollPos < m.maxScroll() {
				m.scrollPos++
			}
		case "left", "h":
			if m.dayFilter == "" && m.currentDay > 0 {
				m.currentDay--
				m.scrollPos = 0
			}
		case "right", "l":
			if m.dayFilter == "" && m.currentDay < 3 {
				m.currentDay++
				m.scrollPos = 0
			}
		case "home":
			m.scrollPos = 0
		case "end":
			m.scrollPos = m.maxScroll()
		case "pageup":
			m.scrollPos -= 10
			if m.scrollPos < 0 {
				m.scrollPos = 0
			}
		case "pagedown":
			m.scrollPos += 10
		}
		return m, nil
	case fetchSuccessMsg:
		m.weatherData = msg.weatherData
		m.loading = false
		if m.currentDay == 1 {
			m.scrollPos = findCurrentRowIndex(m.weatherData.Today)
		}
		return m, nil
	case fetchErrorMsg:
		m.err = msg.err
		m.loading = false
		return m, nil
	}
	return m, nil
}

func main() {
	fs := flag.NewFlagSet("goHeadache", flag.ExitOnError)
	dayFlag := fs.String("day", "", "Filter output by day (yesterday, today, tomorrow, dayafter)")

	if len(os.Args) < 2 {
		fmt.Println("Usage:  goHeadache <area_code> [-day <day>]")
		fmt.Println("\nOptions:")
		fmt.Println("  -day: yesterday, today, tomorrow, or dayafter")
		fmt.Println("\nPlease visit https://geoshape.ex.nii.ac.jp/ka/resource/ to find the appropriate area code.")
		return
	}

	var areaCode string
	var args []string
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, "-") && areaCode == "" {
			areaCode = arg
		} else {
			args = append(args, arg)
		}
	}

	if err := fs.Parse(args); err != nil {
		fmt.Printf("Error parsing flags: %v\n", err)
		return
	}

	if areaCode == "" {
		fmt.Println("Error: Area code is required")
		return
	}

	p := tea.NewProgram(initialModel(areaCode, *dayFlag))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
