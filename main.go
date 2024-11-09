package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"
)

// WeatherData Structs for parsing JSON data
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

// translateWeatherCode - helper function to translate weather codes to descriptions
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

// displayHourlyData - helper function to format hourly data by day
func displayHourlyData(dayName string, data []HourlyData) {
	fmt.Printf("\n%s:\n", dayName)

	// Initialize a new tabwriter for aligning the output columns
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
		// Translate weather code
		weatherDesc := translateWeatherCode(entry.Weather)
		// Print each hourly entry with Pressure Level added
		if _, err := fmt.Fprintf(w, "%s:00\t%s\t%s\t%s\t%s\n", entry.Time, weatherDesc, temp, pressure, entry.PressureLevel); err != nil {
			fmt.Printf("Error writing row to tabwriter: %v\n", err)
			return
		}
	}

	// Flush the tabwriter to ensure all data is printed
	if err := w.Flush(); err != nil {
		fmt.Printf("Error flushing tabwriter: %v\n", err)
	}
}

// Main function
func main() {
	// Check if area code argument is provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: goHeadache <area_code>")
		fmt.Println("Please visit https://geoshape.ex.nii.ac.jp/ka/resource/ to find the appropriate area code.")
		return
	}

	// Get the area code from the command line arguments
	areaCode := os.Args[1]
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
	// The typo 'tommorow' is from the original API response

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

	// Parse each day's data by position in the JSON fields
	if yesterday, exists := rawData[fields[4]]; exists {
		weatherData.Yesterday = parseHourlyData(yesterday)
	}
	if today, exists := rawData[fields[5]]; exists {
		weatherData.Today = parseHourlyData(today)
	}
	if tomorrow, exists := rawData[fields[6]]; exists { // Fixed position for "tommorow" typo
		weatherData.Tomorrow = parseHourlyData(tomorrow)
	}
	if dayAfterTom, exists := rawData[fields[7]]; exists {
		weatherData.DayAfterTom = parseHourlyData(dayAfterTom)
	}

	// Display formatted weather data
	fmt.Printf("Weather Report for %s\n", weatherData.PlaceName)
	fmt.Println("--------------------------------------------")
	displayHourlyData("Yesterday", weatherData.Yesterday)
	displayHourlyData("Today", weatherData.Today)
	displayHourlyData("Tomorrow", weatherData.Tomorrow)
	displayHourlyData("Day After Tomorrow", weatherData.DayAfterTom)
}
