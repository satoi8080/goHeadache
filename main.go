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
	Tomorrow      []HourlyData `json:"tommorow"`
	DayAfterTom   []HourlyData `json:"dayaftertomorrow"`
}

type HourlyData struct {
	Time          string `json:"time"`
	Weather       string `json:"weather"`
	Temp          string `json:"temp"`
	Pressure      string `json:"pressure"`
	PressureLevel string `json:"pressure_level"`
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
		// Print each hourly entry with Pressure Level added
		if _, err := fmt.Fprintf(w, "%s:00\t%s\t%s\t%s\t%s\n", entry.Time, entry.Weather, temp, pressure, entry.PressureLevel); err != nil {
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

	// Parse JSON data into Go struct
	var weatherData WeatherData
	if err := json.Unmarshal(body, &weatherData); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return
	}

	// Display formatted weather data
	fmt.Printf("Weather Report for %s\n", weatherData.PlaceName)
	fmt.Println("--------------------------------------------")
	displayHourlyData("Yesterday", weatherData.Yesterday)
	displayHourlyData("Today", weatherData.Today)
	displayHourlyData("Tomorrow", weatherData.Tomorrow)
	displayHourlyData("Day After Tomorrow", weatherData.DayAfterTom)
}
