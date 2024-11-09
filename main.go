package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
)

// WeatherData and HourlyData structs remain the same...
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

// Helper functions remain the same...
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

func displayHourlyData(dayName string, data []HourlyData) {
	fmt.Printf("\n%s:\n", dayName)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "Time\tWeather\tTemp (°C)\tPressure (hPa)\tPressure Level"); err != nil {
		fmt.Printf("Error writing header to tabwriter: %v\n", err)
		return
	}

	for _, entry := range data {
		temp := entry.Temp
		if temp == "#" {
			temp = "N/A"
		}
		pressure := entry.Pressure
		if pressure == "#" {
			pressure = "N/A"
		}
		weatherDesc := translateWeatherCode(entry.Weather)
		if _, err := fmt.Fprintf(w, "%s:00\t%s\t%s\t%s\t%s\n", entry.Time, weatherDesc, temp, pressure, entry.PressureLevel); err != nil {
			fmt.Printf("Error writing row to tabwriter: %v\n", err)
			return
		}
	}

	if err := w.Flush(); err != nil {
		fmt.Printf("Error flushing tabwriter: %v\n", err)
	}
}

func main() {
	// Create a custom FlagSet
	fs := flag.NewFlagSet("goHeadache", flag.ExitOnError)
	dayFlag := fs.String("day", "", "Filter output by day (yesterday, today, tomorrow, dayafter)")

	// Print usage if no arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: goHeadache [-day <day>] <area_code>")
		fmt.Println("  or:  goHeadache <area_code> [-day <day>]")
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

	url := fmt.Sprintf("https://zutool.jp/api/getweatherstatus/%s", areaCode)

	// Make a GET request
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error making GET request: %v\n", err)
		return
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("Error closing response body: %v\n", cerr)
		}
	}()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %v\n", err)
		return
	}

	// Parse JSON data into a generic map
	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return
	}

	// Initialize the WeatherData struct
	weatherData := WeatherData{}

	// Extract fields by fixed position in the JSON
	fields := []string{"place_name", "place_id", "prefectures_id", "dateTime", "yesterday", "today", "tommorow", "dayaftertomorrow"}

	// Assign values by their expected positions
	if placeName, ok := rawData[fields[0]].(string); ok {
		weatherData.PlaceName = placeName
	}
	if placeID, ok := rawData[fields[1]].(string); ok {
		weatherData.PlaceID = placeID
	}
	if prefecturesID, ok := rawData[fields[2]].(string); ok {
		weatherData.PrefecturesID = prefecturesID
	}
	if dateTime, ok := rawData[fields[3]].(string); ok {
		weatherData.DateTime = dateTime
	}

	// Helper to parse hourly data array
	parseHourlyData := func(data interface{}) []HourlyData {
		var result []HourlyData
		if hourlyArray, ok := data.([]interface{}); ok {
			for _, item := range hourlyArray {
				if hourlyMap, ok := item.(map[string]interface{}); ok {
					entry := HourlyData{
						Time:          fmt.Sprintf("%v", hourlyMap["time"]),
						Weather:       fmt.Sprintf("%v", hourlyMap["weather"]),
						Temp:          fmt.Sprintf("%v", hourlyMap["temp"]),
						Pressure:      fmt.Sprintf("%v", hourlyMap["pressure"]),
						PressureLevel: fmt.Sprintf("%v", hourlyMap["pressure_level"]),
					}
					result = append(result, entry)
				}
			}
		}
		return result
	}

	// Parse each day's data
	if yesterday, exists := rawData[fields[4]]; exists {
		weatherData.Yesterday = parseHourlyData(yesterday)
	}
	if today, exists := rawData[fields[5]]; exists {
		weatherData.Today = parseHourlyData(today)
	}
	if tomorrow, exists := rawData[fields[6]]; exists {
		weatherData.Tomorrow = parseHourlyData(tomorrow)
	}
	if dayAfterTom, exists := rawData[fields[7]]; exists {
		weatherData.DayAfterTom = parseHourlyData(dayAfterTom)
	}

	// Display formatted weather data
	fmt.Printf("Weather Report for %s\n", weatherData.PlaceName)
	fmt.Println("--------------------------------------------")

	// Display data based on the day flag
	switch strings.ToLower(*dayFlag) {
	case "yesterday":
		displayHourlyData("Yesterday", weatherData.Yesterday)
	case "today":
		displayHourlyData("Today", weatherData.Today)
	case "tomorrow":
		displayHourlyData("Tomorrow", weatherData.Tomorrow)
	case "dayafter":
		displayHourlyData("Day After Tomorrow", weatherData.DayAfterTom)
	case "":
		// If no day specified, show all days
		displayHourlyData("Yesterday", weatherData.Yesterday)
		displayHourlyData("Today", weatherData.Today)
		displayHourlyData("Tomorrow", weatherData.Tomorrow)
		displayHourlyData("Day After Tomorrow", weatherData.DayAfterTom)
	default:
		fmt.Printf("Invalid day specified. Please use: yesterday, today, tomorrow, or dayafter\n")
	}
}
